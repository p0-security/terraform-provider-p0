package provider

// copy of internal/provider/auth0_test.go @ 663ab81 (INT-590), renamed for Tailscale
// (INT-686), with the tailnet in place of the tenant.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	tailscaleConnector     = "prod"
	tailscaleTailnet       = "T1234CNTRL"
	tailscaleClientId      = "kAbc1CNTRL"
	tailscaleDiscovery     = "3 tags, 2 subnet routes, 1 service, 0 app connectors, 1 exit node, 0 P0 postures; 4 groups, 0 of them P0's"
	tailscaleSuggestedMode = "direct-write"
	tailscaleDnsName       = "tail1234.ts.net"
)

// Where a Tailscale connector runs, and a tailnet credential and mode that suit it, as
// a customer's Terraform sets them.
type tailscaleCloud struct {
	hosting    string
	credential string
	mode       string
}

// A connector on AWS Lambda, and a tailnet whose federated identity trusts it.
var tailscaleAws = tailscaleCloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-west-2"
  }`,
	credential: `{
    type      = "federated"
    client_id = "kAbc1CNTRL"
  }`,
	mode: "direct-write",
}

// A connector on Cloud Run, and a posture-mode tailnet whose OAuth client's secret is
// in Google Secret Manager.
var tailscaleGcp = tailscaleCloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-west1"
  }`,
	credential: `{
    type      = "oauth-client"
    client_id = "kAbc1CNTRL"
    vault = {
      type       = "gcp-sm"
      project_id = "acme-secrets"
    }
    client_secret_name = "tailscale-acme-oauth"
  }`,
	mode: "posture",
}

func tailscaleProviderConfig(f *tailscaleFake) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

func tailscaleStagedConfig(f *tailscaleFake, hosting string) string {
	return tailscaleProviderConfig(f) + fmt.Sprintf(`
resource "p0_tailscale_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, tailscaleConnector, hosting)
}

// A connector staged and then installed, where the final resource takes the staged
// resource's hosting whole.
func tailscaleConnectorConfig(f *tailscaleFake, hosting string) string {
	return tailscaleStagedConfig(f, hosting) + `
resource "p0_tailscale_connector" "example" {
  id      = p0_tailscale_connector_staged.example.id
  hosting = p0_tailscale_connector_staged.example.hosting
}
`
}

// A connector and a tailnet it acts on.
func tailscaleTailnetConfig(f *tailscaleFake, cloud tailscaleCloud, id string, mode string) string {
	return tailscaleConnectorConfig(f, cloud.hosting) + fmt.Sprintf(`
resource "p0_tailscale_tailnet" "example" {
  id         = %q
  connector  = p0_tailscale_connector.example.id
  credential = %s
  mode       = %q
}
`, id, cloud.credential, mode)
}

// A connector and a tailnet in the mode that suits the cloud, as a customer's Terraform
// installs them.
func tailscaleConfig(f *tailscaleFake, cloud tailscaleCloud) string {
	return tailscaleTailnetConfig(f, cloud, tailscaleTailnet, cloud.mode)
}

// An in-memory stand-in for P0's install API for Tailscale, behaving as P0's Tailscale
// installers do:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - a PUT of an item that exists is refused, so an item can only be staged once
//   - verify and configure refuse a request that changes what P0 fixed when it staged
//     the item: a tailnet's connector, credential and mode
//   - a tailnet can only be staged for a connector that is installed
//   - a tailnet's generated fields are empty at stage, and verifying it stores what
//     discovery found
//   - verifying a connector on Cloud Run resolves its URL
//   - a connector's metadata names the identity it runs as
//   - unknown items are 404, and DELETE answers 204 with no body
//
// With failVerify set, verifying a tailnet fails as it does when the connector can't
// use the credential.
type tailscaleFake struct {
	server *httptest.Server

	mu         sync.Mutex
	installed  bool
	items      map[tailscaleItemKey]map[string]any
	failVerify bool
}

// A Tailscale item: a connector or a tailnet, by its ID.
type tailscaleItemKey struct {
	component, id string
}

func newTailscaleFake(t *testing.T) *tailscaleFake {
	f := &tailscaleFake{items: map[tailscaleItemKey]map[string]any{}}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/tailscale/config"
	item := base + "/{component}/{id}"
	mux.HandleFunc("POST "+base, f.postConfig)
	mux.HandleFunc("PUT "+item, f.step("stage", nil))
	mux.HandleFunc("POST "+item+"/verify", f.step("configure", nil))
	mux.HandleFunc("POST "+item+"/configure", f.step("installed", []string{"configure", "installed"}))
	mux.HandleFunc("GET "+item, f.getItem)
	mux.HandleFunc("DELETE "+item, f.deleteItem)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to the fake P0 API: %s %s", r.Method, r.URL.Path)
		notFound(w)
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func tailscaleItemKeyOf(r *http.Request) tailscaleItemKey {
	return tailscaleItemKey{r.PathValue("component"), r.PathValue("id")}
}

func (f *tailscaleFake) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.installed {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.installed = true
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *tailscaleFake) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		key := tailscaleItemKeyOf(r)
		if !f.installed {
			notFound(w)
			return
		}
		previous, exists := f.items[key]
		// Only staging may create an item.
		if !exists && nextState != "stage" {
			notFound(w)
			return
		}
		state, _ := previous["state"].(string)
		if expectedStates != nil && !slices.Contains(expectedStates, state) {
			http.Error(w, `{"error":"Invalid integration state"}`, http.StatusBadRequest)
			return
		}
		if status, message := f.refuse(key, nextState, previous, body); status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
			return
		}

		updated := deepMerge(previous, body)
		// P0 never lets a request change an item's label.
		if label, ok := previous["label"]; ok {
			updated["label"] = label
		} else {
			updated["label"] = key.id
		}
		updated["state"] = nextState
		normalizeTailscaleItem(key, updated)

		f.items[key] = updated
		writeTailscaleItem(w, key, updated)
	}
}

// The fields P0 fixes when it stages a tailnet's item. (A connector's hosting is fixed
// too, but P0 adds the Cloud Run URL to it, which no request carries, so this fake
// leaves it out.)
var tailscaleFixedFields = map[string][]string{
	"tailnet": {"connector", "credential", "mode"},
}

// Refuses a step the way P0's Tailscale installers do: with an HTTP status and P0's
// message, or 0 to accept it. previous is the stored item, nil when there is none, and
// body is the request's. Call it with f.mu held.
func (f *tailscaleFake) refuse(key tailscaleItemKey, nextState string, previous, body map[string]any) (int, string) {
	if nextState == "stage" && previous != nil {
		return http.StatusConflict, fmt.Sprintf("The %s %s is already added. To change it, remove %s and add it again.", key.component, key.id, key.id)
	}
	if nextState != "stage" {
		for _, field := range tailscaleFixedFields[key.component] {
			if !reflect.DeepEqual(body[field], previous[field]) {
				return http.StatusBadRequest, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
			}
		}
	}
	switch {
	case key.component == "tailnet" && nextState == "stage":
		connector, _ := body["connector"].(string)
		if f.items[tailscaleItemKey{"connector", connector}]["state"] != "installed" {
			return http.StatusBadRequest, "Pick the connector that acts on this tailnet. Install a Tailscale connector first if there's none."
		}
	case key.component == "tailnet" && nextState == "configure" && f.failVerify:
		return http.StatusUnprocessableEntity, "The connector couldn't get a Tailscale token with the credential."
	}
	return 0, ""
}

// Adds the fields that P0's Tailscale installers assign to an item.
func normalizeTailscaleItem(key tailscaleItemKey, item map[string]any) {
	staged := item["state"] == "stage"
	switch key.component {
	case "connector":
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" && !staged {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	case "tailnet":
		item["discovery"], item["suggestedMode"], item["dnsName"] = "", "", ""
		if !staged {
			item["discovery"], item["suggestedMode"], item["dnsName"] = tailscaleDiscovery, tailscaleSuggestedMode, tailscaleDnsName
		}
	}
}

// Computes an item's metadata, the identity a connector runs as. Nil omits the key, as
// P0 does.
func tailscaleItemMetadata(key tailscaleItemKey, item map[string]any) map[string]any {
	hosting, ok := item["hosting"].(map[string]any)
	if key.component != "connector" || !ok {
		return nil
	}
	name := hosting["connectorName"]
	if hosting["type"] == "gcp" {
		return map[string]any{"connectorIdentity": map[string]any{
			"type":                "gcp",
			"projectId":           hosting["projectId"],
			"serviceAccountEmail": fmt.Sprintf("%s@%s.iam.gserviceaccount.com", name, hosting["projectId"]),
			"serviceAccountId":    name,
		}}
	}
	return map[string]any{"connectorIdentity": map[string]any{
		"type":      "aws",
		"accountId": hosting["accountId"],
		"roleArn":   fmt.Sprintf("arn:aws:iam::%s:role/%s", hosting["accountId"], name),
		"roleName":  name,
	}}
}

func (f *tailscaleFake) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := tailscaleItemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	writeTailscaleItem(w, key, stored)
}

func (f *tailscaleFake) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := tailscaleItemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeTailscaleItem(w http.ResponseWriter, key tailscaleItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := tailscaleItemMetadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *tailscaleFake) item(component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[tailscaleItemKey{component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *tailscaleFake) update(component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[tailscaleItemKey{component, id}])
}

// The number of items P0 has, of every component.
func (f *tailscaleFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.items)
}

func checkTailscaleDestroyed(f *tailscaleFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if n := f.count(); n != 0 {
			return fmt.Errorf("%d items were not removed from P0", n)
		}
		return nil
	}
}

// The full chain on AWS, with a federated direct-write tailnet: the connector is staged once and then only verified and configured, since P0 refuses
// to stage an item twice.
func TestTailscaleOnAws(t *testing.T) {
	f := newTailscaleFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkTailscaleDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: tailscaleConfig(f, tailscaleAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_tailscale_connector_staged.example", "connector_identity.role_arn", "arn:aws:iam::123456789012:role/p0-tailscale-prod"),
					resource.TestCheckResourceAttr("p0_tailscale_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "credential.type", "federated"),
					resource.TestCheckNoResourceAttr("p0_tailscale_tailnet.example", "credential.vault"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "discovery", tailscaleDiscovery),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "suggested_mode", tailscaleSuggestedMode),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "dns_name", tailscaleDnsName),
					func(*terraform.State) error {
						item := f.item("tailnet", tailscaleTailnet)
						credential, _ := item["credential"].(map[string]any)
						mode, _ := item["mode"].(map[string]any)
						if item["connector"] != tailscaleConnector || credential["type"] != "federated" || credential["clientId"] != tailscaleClientId ||
							len(credential) != 2 || mode["type"] != "direct-write" {
							return fmt.Errorf("P0 item does not hold the configured tailnet: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_tailscale_tailnet.example",
				ImportState:       true,
				ImportStateId:     tailscaleTailnet,
				ImportStateVerify: true,
			},
		},
	})
}

// The full chain on Google Cloud, with a posture-mode tailnet whose OAuth client's
// secret is in Google Secret Manager.
func TestTailscaleOnGoogleCloud(t *testing.T) {
	f := newTailscaleFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkTailscaleDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: tailscaleConfig(f, tailscaleGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_tailscale_connector.example", "hosting.connector_service_uri", "https://p0-tailscale-prod-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(
						"p0_tailscale_connector.example", "connector_identity.service_account_email",
						"p0-tailscale-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "mode", "posture"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "credential.vault.project_id", "acme-secrets"),
					func(*terraform.State) error {
						item := f.item("tailnet", tailscaleTailnet)
						credential, _ := item["credential"].(map[string]any)
						vault, _ := credential["vault"].(map[string]any)
						if credential["type"] != "oauth-client" || credential["clientSecretName"] != "tailscale-acme-oauth" ||
							vault["type"] != "gcp-sm" || vault["install"] != "acme-secrets" || len(vault) != 2 {
							return fmt.Errorf("P0 item does not hold the configured tailnet: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_tailscale_tailnet.example",
				ImportState:       true,
				ImportStateId:     tailscaleTailnet,
				ImportStateVerify: true,
			},
		},
	})
}

// An installed tailnet's connector, credential and mode can't change: the plan is
// refused, and P0's item stays as it was. A new credential type with a vault is
// refused too, though no field of a vault was in state. A new ID is another tailnet,
// which replaces this one. The unit tests cover each field.
func TestTailscaleTailnetRefusesAFixedChange(t *testing.T) {
	f := newTailscaleFake(t)
	withCredential := func(credential string) tailscaleCloud {
		cloud := tailscaleAws
		cloud.credential = credential
		return cloud
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkTailscaleDestroyed(f),
		Steps: []resource.TestStep{
			{Config: tailscaleTailnetConfig(f, tailscaleAws, tailscaleTailnet, "direct-write")},
			{
				Config:      tailscaleTailnetConfig(f, tailscaleAws, tailscaleTailnet, "posture"),
				ExpectError: regexp.MustCompile(`(?s)A tailnet's mode can't change.*revoke the tailnet's\s+grants`),
			},
			{
				Config:      tailscaleTailnetConfig(f, withCredential(tailscaleGcp.credential), tailscaleTailnet, "direct-write"),
				ExpectError: regexp.MustCompile(`A tailnet's credential can't change`),
			},
			{
				PreConfig: func() {
					mode, _ := f.item("tailnet", tailscaleTailnet)["mode"].(map[string]any)
					if mode["type"] != "direct-write" {
						t.Errorf("P0 item's mode after the refused plans = %v; want it unchanged", mode)
					}
				},
				Config: tailscaleTailnetConfig(f, tailscaleAws, "T5678CNTRL", "posture"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_tailscale_tailnet.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.item("tailnet", tailscaleTailnet); item != nil {
						return fmt.Errorf("the old tailnet was not removed from P0: %v", item)
					}
					if item := f.item("tailnet", "T5678CNTRL"); item["state"] != "installed" {
						return fmt.Errorf("the new tailnet is not installed: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestTailscaleConnectorHostingReplacesBoth(t *testing.T) {
	f := newTailscaleFake(t)
	moved := `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-tailscale-prod"
    connector_region = "europe-west1"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: tailscaleConnectorConfig(f, tailscaleGcp.hosting)},
			{
				Config: tailscaleConnectorConfig(f, moved),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_tailscale_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_tailscale_connector.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					item := f.item("connector", tailscaleConnector)
					hosting, _ := item["hosting"].(map[string]any)
					if item["state"] != "installed" || hosting["connectorRegion"] != "europe-west1" {
						return fmt.Errorf("P0 item is not the connector installed where it moved to: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// Destroying only a final connector resource returns its item to "stage", so the
// staged resource still owns it, and a later apply installs it again.
func TestTailscaleConnectorRollback(t *testing.T) {
	f := newTailscaleFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkTailscaleDestroyed(f),
		Steps: []resource.TestStep{
			{Config: tailscaleConnectorConfig(f, tailscaleGcp.hosting)},
			{
				Config: tailscaleStagedConfig(f, tailscaleGcp.hosting),
				Check: func(*terraform.State) error {
					item := f.item("connector", tailscaleConnector)
					hosting, _ := item["hosting"].(map[string]any)
					if item == nil || item["state"] != "stage" || hosting["connectorName"] != "p0-tailscale-prod" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: tailscaleConnectorConfig(f, tailscaleGcp.hosting),
				Check:  resource.TestCheckResourceAttr("p0_tailscale_connector.example", "state", "installed"),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestTailscaleUnfinishedInstallConverges(t *testing.T) {
	f := newTailscaleFake(t)
	restage := func(component, id string) {
		f.update(component, id, func(item map[string]any) {
			item["state"] = "stage"
			item["discovery"], item["suggestedMode"], item["dnsName"] = "", "", ""
		})
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: tailscaleConfig(f, tailscaleAws)},
			{
				PreConfig: func() { restage("tailnet", tailscaleTailnet) },
				Config:    tailscaleConfig(f, tailscaleAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_tailscale_tailnet.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("p0_tailscale_connector.example", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "discovery", tailscaleDiscovery),
				),
			},
		},
	})
}

// A tailnet whose install check fails is left staged in P0 and tainted in Terraform.
// P0 refuses to stage it again, so the next apply replaces it: it removes the item,
// then stages and installs it.
func TestTailscaleTailnetFailedCheckIsReplaced(t *testing.T) {
	f := newTailscaleFake(t)
	f.failVerify = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      tailscaleTailnetConfig(f, tailscaleAws, tailscaleTailnet, "direct-write"),
				ExpectError: regexp.MustCompile(`(?s)Tailscale tailnet install check failed.*couldn't get a\s+Tailscale token`),
			},
			{
				PreConfig: func() {
					if item := f.item("tailnet", tailscaleTailnet); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failVerify = false
				},
				Config: tailscaleTailnetConfig(f, tailscaleAws, tailscaleTailnet, "direct-write"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_tailscale_tailnet.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_tailscale_tailnet.example", "state", "installed"),
			},
		},
	})
}

// An item that is already added in P0 can't be created, only imported.
func TestTailscaleRefusesAnItemThatExists(t *testing.T) {
	f := newTailscaleFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: tailscaleConfig(f, tailscaleAws)},
			{
				Config: tailscaleConfig(f, tailscaleAws) + fmt.Sprintf(`
resource "p0_tailscale_tailnet" "duplicate" {
  id         = %q
  connector  = p0_tailscale_connector.example.id
  credential = %s
  mode       = %q
}
`, tailscaleTailnet, tailscaleAws.credential, tailscaleAws.mode),
				ExpectError: regexp.MustCompile(`(?s)Could not stage tailnet component.*is already added`),
			},
		},
	})
}

// The checks that reach Terraform at plan time, before the provider calls P0.
func TestTailscaleValidation(t *testing.T) {
	tailnet := func(id, credential, mode string) string {
		return fmt.Sprintf(`
resource "p0_tailscale_tailnet" "example" {
  id         = %q
  connector  = "prod"
  credential = %s
  mode       = %q
}
`, id, credential, mode)
	}
	staged := func(hosting string) string {
		return fmt.Sprintf(`
resource "p0_tailscale_connector_staged" "example" {
  id      = "prod"
  hosting = %s
}
`, hosting)
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"GovCloud connector", staged(`{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-gov-west-1"
  }`), "P0's Tailscale integration supports only"},
		{"tailnet's DNS name", tailnet("tail1234.ts.net", tailscaleAws.credential, "direct-write"), `Enter the Tailnet ID`},
		{"unknown mode", tailnet(tailscaleTailnet, tailscaleAws.credential, "read-only"), `value must be one of`},
		{"client ID with a space", tailnet(tailscaleTailnet, `{
    type      = "federated"
    client_id = "abc def"
  }`, "direct-write"), `Enter\s+the\s+client\s+ID`},
		{"federated identity with a vault", tailnet(tailscaleTailnet, `{
    type      = "federated"
    client_id = "kAbc1CNTRL"
    vault = {
      type       = "gcp-sm"
      project_id = "acme-secrets"
    }
  }`, "direct-write"), `may only be set when 'credential.type' is\s+"oauth-client"`},
		{"OAuth client without a secret name", tailnet(tailscaleTailnet, `{
    type      = "oauth-client"
    client_id = "kAbc1CNTRL"
    vault = {
      type       = "gcp-sm"
      project_id = "acme-secrets"
    }
  }`, "direct-write"), `'credential.client_secret_name' is required`},
		{"secret ARN", tailnet(tailscaleTailnet, `{
    type      = "oauth-client"
    client_id = "kAbc1CNTRL"
    vault = {
      type           = "aws-sm"
      account_id     = "123456789012"
      secrets_region = "us-west-2"
    }
    client_secret_name = "arn:aws:secretsmanager:us-west-2:123456789012:secret:tailscale"
  }`, "direct-write"), "doesn't look like a secret name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTailscaleFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tailscaleProviderConfig(f) + tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}
