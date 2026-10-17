package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	onePasswordConnector = "prod"
	onePasswordAccountId = "acme.1password.com"
	onePasswordDomains   = `^example\.com$`
)

// Where a 1Password connector runs, and the secret manager of the same cloud, as a
// customer's Terraform sets them.
type onePasswordCloud struct {
	hosting       string
	secretManager string
	secret        string
}

var onePasswordAws = onePasswordCloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-1password-prod"
    connector_region = "us-west-2"
  }`,
	secretManager: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
	secret: "1password/engineering/token",
}

var onePasswordGcp = onePasswordCloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-1password-prod"
    connector_region = "us-west1"
  }`,
	secretManager: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
	secret: "1password-engineering-token",
}

func onePasswordProviderConfig(f *onePasswordFake) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

func onePasswordStagedConfig(f *onePasswordFake, hosting string) string {
	return onePasswordProviderConfig(f) + fmt.Sprintf(`
resource "p0_1password_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, onePasswordConnector, hosting)
}

// A connector staged and then installed, where the final resource takes the staged
// resource's hosting whole.
func onePasswordConnectorConfig(f *onePasswordFake, hosting string) string {
	return onePasswordStagedConfig(f, hosting) + `
resource "p0_1password_connector" "example" {
  id      = p0_1password_connector_staged.example.id
  hosting = p0_1password_connector_staged.example.hosting
}
`
}

// A connector and a 1Password account whose items it shares, as a customer's Terraform installs them.
func onePasswordConfig(f *onePasswordFake, cloud onePasswordCloud) string {
	return onePasswordConfigWith(f, cloud, "")
}

// onePasswordConfig, with extra lines in the account's block.
func onePasswordConfigWith(f *onePasswordFake, cloud onePasswordCloud, extra string) string {
	return onePasswordConnectorConfig(f, cloud.hosting) + fmt.Sprintf(`
resource "p0_1password_account" "example" {
  id        = %q
  connector = p0_1password_connector.example.id

  secret_manager = %s
  token_secret_name = %q
%s}
`, onePasswordAccountId, cloud.secretManager, cloud.secret, extra)
}

// An in-memory stand-in for P0's install API for 1Password, behaving as P0's
// 1Password installers do:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - a PUT of an item that exists is refused, so an item can only be staged once
//   - an account can only be staged for a connector that is installed
//   - verifying a connector on Cloud Run resolves its URL
//   - verifying an account reads the allowed domains back from its connector
//   - a connector's metadata names the identity it runs as
//   - unknown items are 404, and DELETE answers 204 with no body
//
// With failVerify set, verifying an account fails as it does when the connector
// can't read the token's secret.
type onePasswordFake struct {
	server *httptest.Server

	mu         sync.Mutex
	installed  bool
	items      map[onePasswordItemKey]map[string]any
	failVerify bool
}

// A 1Password item: a connector or an account, by its ID.
type onePasswordItemKey struct {
	component, id string
}

func newOnePasswordFake(t *testing.T) *onePasswordFake {
	f := &onePasswordFake{items: map[onePasswordItemKey]map[string]any{}}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/1password/config"
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

func onePasswordItemKeyOf(r *http.Request) onePasswordItemKey {
	return onePasswordItemKey{r.PathValue("component"), r.PathValue("id")}
}

func (f *onePasswordFake) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.installed {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.installed = true
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *onePasswordFake) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		key := onePasswordItemKeyOf(r)
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
		normalizeOnePasswordItem(key, updated)

		f.items[key] = updated
		writeOnePasswordItem(w, key, updated)
	}
}

// Refuses a step the way P0's 1Password installers do: with an HTTP status and P0's
// message, or 0 to accept it. previous is the stored item, nil when there is none, and
// body is the request's. Call it with f.mu held.
func (f *onePasswordFake) refuse(key onePasswordItemKey, nextState string, previous, body map[string]any) (int, string) {
	switch {
	case nextState == "stage" && previous != nil:
		return http.StatusConflict, fmt.Sprintf("The %s %s is already added. To change it, remove %s and add it again.", key.component, key.id, key.id)
	case key.component == "account" && nextState == "stage":
		connector, _ := body["connector"].(string)
		if f.items[onePasswordItemKey{"connector", connector}]["state"] != "installed" {
			return http.StatusBadRequest, "Pick the connector that shares this account's items. Install a 1Password connector first if there's none."
		}
	case key.component == "account" && nextState == "configure" && f.failVerify:
		return http.StatusUnprocessableEntity, "The connector isn't allowed to read the secret."
	}
	return 0, ""
}

// Adds the fields that P0's 1Password installers add when they verify an item.
func normalizeOnePasswordItem(key onePasswordItemKey, item map[string]any) {
	if item["state"] == "stage" {
		return
	}
	switch key.component {
	case "connector":
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	case "account":
		item["allowedDomains"] = onePasswordDomains
	}
}

// Computes an item's metadata, the identity a connector runs as. Nil omits the key, as
// P0 does.
func onePasswordItemMetadata(key onePasswordItemKey, item map[string]any) map[string]any {
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

func (f *onePasswordFake) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := onePasswordItemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	writeOnePasswordItem(w, key, stored)
}

func (f *onePasswordFake) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := onePasswordItemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeOnePasswordItem(w http.ResponseWriter, key onePasswordItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := onePasswordItemMetadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *onePasswordFake) item(component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[onePasswordItemKey{component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *onePasswordFake) update(component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[onePasswordItemKey{component, id}])
}

func (f *onePasswordFake) connector() map[string]any {
	return f.item("connector", onePasswordConnector)
}

func (f *onePasswordFake) account() map[string]any {
	return f.item("account", onePasswordAccountId)
}

func checkOnePasswordDestroyed(f *onePasswordFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		if item := f.account(); item != nil {
			return fmt.Errorf("the account was not removed from P0: %v", item)
		}
		return nil
	}
}

// The full chain on Google Cloud: the connector is staged once and then only verified
// and configured, since P0 refuses to stage an item twice.
func TestOnePasswordOnGoogleCloud(t *testing.T) {
	f := newOnePasswordFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: onePasswordConfig(f, onePasswordGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_1password_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_1password_connector_staged.example", "connector_identity.type", "gcp"),
					resource.TestCheckResourceAttr("p0_1password_connector_staged.example", "connector_identity.service_account_id", "p0-1password-prod"),
					resource.TestCheckResourceAttr(
						"p0_1password_connector_staged.example", "connector_identity.service_account_email",
						"p0-1password-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckNoResourceAttr("p0_1password_connector_staged.example", "connector_identity.role_arn"),
					resource.TestCheckNoResourceAttr("p0_1password_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "hosting.project_id", "acme-prod"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "hosting.connector_service_uri", "https://p0-1password-prod-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(
						"p0_1password_connector.example", "connector_identity.service_account_email",
						"p0-1password-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "allowed_domains", onePasswordDomains),
					func(*terraform.State) error {
						item := f.account()
						secretManager, _ := item["secretManager"].(map[string]any)
						if item["connector"] != onePasswordConnector || item["tokenSecretName"] != onePasswordGcp.secret ||
							secretManager["type"] != "gcp-sm" || secretManager["install"] != "acme-secrets" || len(secretManager) != 2 {
							return fmt.Errorf("P0 item does not hold the configured account: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_1password_connector_staged.example",
				ImportState:       true,
				ImportStateId:     onePasswordConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      "p0_1password_connector.example",
				ImportState:       true,
				ImportStateId:     onePasswordConnector,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_1password_account.example",
				ImportState:       true,
				ImportStateId:     onePasswordAccountId,
				ImportStateVerify: true,
			},
		},
	})
}

// The full chain on AWS, where P0 derives the connector's address, and a secret that
// has a region.
func TestOnePasswordOnAws(t *testing.T) {
	f := newOnePasswordFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: onePasswordConfig(f, onePasswordAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_1password_connector_staged.example", "connector_identity.type", "aws"),
					resource.TestCheckResourceAttr("p0_1password_connector_staged.example", "connector_identity.role_name", "p0-1password-prod"),
					resource.TestCheckResourceAttr(
						"p0_1password_connector_staged.example", "connector_identity.role_arn",
						"arn:aws:iam::123456789012:role/p0-1password-prod",
					),
					resource.TestCheckNoResourceAttr("p0_1password_connector_staged.example", "connector_identity.service_account_email"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_1password_connector.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "secret_manager.secrets_region", "us-west-2"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "allowed_domains", onePasswordDomains),
					// One-time links are off, and the request limit is one per item, unless the
					// configuration says otherwise.
					resource.TestCheckResourceAttr("p0_1password_account.example", "one_time_links", "false"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "request_limit", "item"),
					func(*terraform.State) error {
						item := f.account()
						secretManager, _ := item["secretManager"].(map[string]any)
						requestLimit, _ := item["requestLimit"].(map[string]any)
						if item["tokenSecretName"] != onePasswordAws.secret || item["oneTimeLinks"] != false || requestLimit["type"] != "item" || secretManager["type"] != "aws-sm" ||
							secretManager["install"] != "123456789012" || secretManager["secretsRegion"] != "us-west-2" {
							return fmt.Errorf("P0 item does not hold the configured account: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_1password_account.example",
				ImportState:       true,
				ImportStateId:     onePasswordAccountId,
				ImportStateVerify: true,
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestOnePasswordConnectorHostingReplacesBoth(t *testing.T) {
	f := newOnePasswordFake(t)
	moved := `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-1password-prod"
    connector_region = "europe-west1"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: onePasswordConnectorConfig(f, onePasswordGcp.hosting)},
			{
				Config: onePasswordConnectorConfig(f, moved),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_1password_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_1password_connector.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					item := f.connector()
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
func TestOnePasswordConnectorRollback(t *testing.T) {
	f := newOnePasswordFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			{Config: onePasswordConnectorConfig(f, onePasswordGcp.hosting)},
			{
				Config: onePasswordStagedConfig(f, onePasswordGcp.hosting),
				Check: func(*terraform.State) error {
					item := f.connector()
					hosting, _ := item["hosting"].(map[string]any)
					if item == nil || item["state"] != "stage" || hosting["connectorName"] != "p0-1password-prod" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: onePasswordConnectorConfig(f, onePasswordGcp.hosting),
				Check:  resource.TestCheckResourceAttr("p0_1password_connector.example", "state", "installed"),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestOnePasswordUnfinishedInstallConverges(t *testing.T) {
	f := newOnePasswordFake(t)
	restage := func(component, id string) {
		f.update(component, id, func(item map[string]any) {
			item["state"] = "stage"
			delete(item, "allowedDomains")
			if hosting, ok := item["hosting"].(map[string]any); ok {
				delete(hosting, "connectorServiceUri")
			}
		})
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: onePasswordConfig(f, onePasswordGcp)},
			{
				PreConfig: func() { restage("connector", onePasswordConnector) },
				Config:    onePasswordConfig(f, onePasswordGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_1password_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_1password_connector.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_1password_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_connector.example", "hosting.connector_service_uri", "https://p0-1password-prod-abc123-uw.a.run.app"),
				),
			},
			{
				PreConfig: func() { restage("account", onePasswordAccountId) },
				Config:    onePasswordConfig(f, onePasswordGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_account.example", "allowed_domains", onePasswordDomains),
				),
			},
		},
	})
}

// One-time links can be on from the start, and change in place after: the update
// configures the account without verifying it again, so it doesn't call the connector,
// whose check fails throughout. A change made in P0 shows as drift.
func TestOnePasswordAccountOneTimeLinks(t *testing.T) {
	f := newOnePasswordFake(t)
	oneTimeLinks := func(want bool) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("p0_1password_account.example", "one_time_links", fmt.Sprint(want)),
			resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
			func(*terraform.State) error {
				if item := f.account(); item["oneTimeLinks"] != want {
					return fmt.Errorf("P0 item oneTimeLinks = %v; want %v", item["oneTimeLinks"], want)
				}
				return nil
			},
		)
	}
	inPlace := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionUpdate),
		},
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: onePasswordConfigWith(f, onePasswordAws, "  one_time_links = true\n"),
				Check:  oneTimeLinks(true),
			},
			{
				PreConfig:        func() { f.failVerify = true },
				Config:           onePasswordConfigWith(f, onePasswordAws, "  one_time_links = false\n"),
				ConfigPlanChecks: inPlace,
				Check:            oneTimeLinks(false),
			},
			{
				Config:           onePasswordConfig(f, onePasswordAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				PreConfig: func() {
					f.update("account", onePasswordAccountId, func(item map[string]any) { item["oneTimeLinks"] = true })
				},
				Config:           onePasswordConfig(f, onePasswordAws),
				ConfigPlanChecks: inPlace,
				Check: resource.ComposeAggregateTestCheckFunc(
					oneTimeLinks(false),
					func(*terraform.State) error { f.failVerify = false; return nil },
				),
			},
		},
	})
}

// The request limit can be set from the start, and change in place after, as one-time
// links do: the update configures the account without calling the connector, whose
// check fails throughout. Leaving it out of the configuration sets it back to one per
// item.
func TestOnePasswordAccountRequestLimit(t *testing.T) {
	f := newOnePasswordFake(t)
	requestLimit := func(want string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("p0_1password_account.example", "request_limit", want),
			resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
			func(*terraform.State) error {
				limit, _ := f.account()["requestLimit"].(map[string]any)
				if limit["type"] != want || len(limit) != 1 {
					return fmt.Errorf("P0 item requestLimit = %v; want {type: %s}", limit, want)
				}
				return nil
			},
		)
	}
	inPlace := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionUpdate),
		},
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: onePasswordConfigWith(f, onePasswordAws, "  request_limit = \"user\"\n"),
				Check:  requestLimit("user"),
			},
			{
				PreConfig:        func() { f.failVerify = true },
				Config:           onePasswordConfigWith(f, onePasswordAws, "  request_limit = \"vault\"\n"),
				ConfigPlanChecks: inPlace,
				Check:            requestLimit("vault"),
			},
			{
				Config:           onePasswordConfig(f, onePasswordAws),
				ConfigPlanChecks: inPlace,
				Check: resource.ComposeAggregateTestCheckFunc(
					requestLimit("item"),
					func(*terraform.State) error { f.failVerify = false; return nil },
				),
			},
		},
	})
}

// An account whose install check fails is left staged in P0 and tainted in
// Terraform. P0 refuses to stage it again, so the next apply replaces it: it removes
// the item, then stages and installs it.
func TestOnePasswordAccountFailedCheckIsReplaced(t *testing.T) {
	f := newOnePasswordFake(t)
	f.failVerify = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      onePasswordConfig(f, onePasswordAws),
				ExpectError: regexp.MustCompile(`(?s)1Password account install check failed.*isn't allowed to read the\s+secret`),
			},
			{
				PreConfig: func() {
					if item := f.account(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failVerify = false
				},
				Config: onePasswordConfig(f, onePasswordAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
			},
		},
	})
}

// P0 adds an account only for a connector that is installed. Naming one that is
// only staged fails the apply with P0's message and saves nothing, so the account
// is created once its connector is installed.
func TestOnePasswordAccountNeedsAnInstalledConnector(t *testing.T) {
	f := newOnePasswordFake(t)
	account := fmt.Sprintf(`
resource "p0_1password_account" "example" {
  id        = %q
  connector = p0_1password_connector_staged.example.id

  secret_manager = %s
  token_secret_name = %q
}
`, onePasswordAccountId, onePasswordAws.secretManager, onePasswordAws.secret)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      onePasswordStagedConfig(f, onePasswordAws.hosting) + account,
				ExpectError: regexp.MustCompile(`(?s)Could not stage account component.*Pick the connector`),
			},
			{
				PreConfig: func() {
					if item := f.account(); item != nil {
						t.Errorf("P0 item after the refused account = %v; want none", item)
					}
				},
				Config: onePasswordConfig(f, onePasswordAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_1password_account.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_1password_account.example", "state", "installed"),
			},
		},
	})
}

// An item that is already added in P0 can't be created, only imported.
func TestOnePasswordRefusesAnItemThatExists(t *testing.T) {
	f := newOnePasswordFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: onePasswordStagedConfig(f, onePasswordAws.hosting)},
			{
				Config: onePasswordStagedConfig(f, onePasswordAws.hosting) + fmt.Sprintf(`
resource "p0_1password_connector_staged" "duplicate" {
  id      = %q
  hosting = %s
}
`, onePasswordConnector, onePasswordAws.hosting),
				ExpectError: regexp.MustCompile(`(?s)Could not stage connector component.*is already added`),
			},
		},
	})
}

func TestOnePasswordValidation(t *testing.T) {
	awsHosting := func(field, value string) string {
		fields := map[string]string{"account_id": "123456789012", "connector_name": "p0-1password-prod", "connector_region": "us-west-2"}
		fields[field] = value
		return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
	}
	account := func(id, secretManager, secret string) string {
		return fmt.Sprintf(`
resource "p0_1password_account" "example" {
  id                = %q
  connector         = "prod"
  secret_manager = %s
  token_secret_name = %q
}
`, id, secretManager, secret)
	}
	accountWith := func(id, extra string) string {
		return fmt.Sprintf(`
resource "p0_1password_account" "example" {
  id                = %q
  connector         = "prod"
  secret_manager = %s
  token_secret_name = %q
%s}
`, id, onePasswordAws.secretManager, onePasswordAws.secret, extra)
	}
	staged := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_1password_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"connector id with capitals", staged("Prod", onePasswordAws.hosting), "lowercase letters"},
		{"unknown request limit", accountWith(onePasswordAccountId, "  request_limit = \"account\"\n"), "value must be one of"},
		{"connector name ECR wouldn't take", staged("prod", awsHosting("connector_name", "p0--1password")), "Use 2 to 64 characters"},
		{"GovCloud connector", staged("prod", awsHosting("connector_region", "us-gov-west-1")), "P0's 1Password integration supports only"},
		{
			"connector name too long for a service account",
			staged("prod", `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-1password-connector-for-production"
    connector_region = "us-west1"
  }`),
			"Use 6 to 30 characters",
		},
		{
			"project on AWS hosting",
			staged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-1password-prod"
    connector_region = "us-west-2"
  }`),
			"Missing AWS account ID",
		},
		{"sign-in address with capitals", account("Acme.1password.com", onePasswordAws.secretManager, onePasswordAws.secret), "sign-in address"},
		{"sign-in address as a URL", account("https://acme.1password.com", onePasswordAws.secretManager, onePasswordAws.secret), "sign-in address"},
		{"vault ID in place of the sign-in address", account("3fz7kq2xmw4vjn6rbt5hlpd8yc", onePasswordAws.secretManager, onePasswordAws.secret), "sign-in address"},
		{"secret ARN", account(onePasswordAccountId, onePasswordAws.secretManager, "arn:aws:secretsmanager:us-west-2:123456789012:secret:token"), "doesn't look like a secret name"},
		{"AWS secret name in Google Secret Manager", account(onePasswordAccountId, onePasswordGcp.secretManager, onePasswordAws.secret), "doesn't look like a secret name"},
		{"service account token in place of its secret's name", account(onePasswordAccountId, onePasswordAws.secretManager, "ops_eyJzaWduSW5BZGRyZXNzIjoiYWNtZS4xcGFzc3dvcmQuY29tIn0"), "That's a 1Password service account token"},
		{
			"region on a Google secret manager",
			account(onePasswordAccountId, `{
    type           = "gcp-sm"
    project_id     = "acme-secrets"
    secrets_region = "us-west-2"
  }`, onePasswordGcp.secret),
			"Unexpected secrets region",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOnePasswordFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      onePasswordProviderConfig(f) + tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}

// The variables that name what TestAcc1Password installs: a 1Password connector
// already deployed on AWS Lambda where they say, running as the role P0 names after
// it, and an account whose service account token is in a secret that the connector's role
// can read, in the connector's account. P0 validates both through the connector, so
// they must be real.
func onePasswordAccEnv(install *onePasswordAccInstall) map[string]*string {
	return map[string]*string{
		"P0_1PASSWORD_AWS_ACCOUNT_ID":    &install.accountId,
		"P0_1PASSWORD_CONNECTOR_NAME":    &install.connectorName,
		"P0_1PASSWORD_CONNECTOR_REGION":  &install.connectorRegion,
		"P0_1PASSWORD_TOKEN_SECRET_NAME": &install.tokenSecretName,
		"P0_1PASSWORD_SECRETS_REGION":    &install.secretsRegion,
		"P0_1PASSWORD_SIGN_IN_ADDRESS":   &install.signInAddress,
		"P0_1PASSWORD_CONNECTOR_ID":      &install.connectorId,
	}
}

type onePasswordAccInstall struct {
	accountId, connectorName, connectorRegion     string
	signInAddress, tokenSecretName, secretsRegion string
	connectorId                                   string
}

// TestAcc1Password installs into the P0 organization P0_ORG, served at P0_HOST (the
// provider's default host unless set), with the API token P0_API_TOKEN. Use an
// organization set aside for tests: it creates and deletes real installs.
const (
	onePasswordAccOrgEnv      = "P0_ORG"
	onePasswordAccHostEnv     = "P0_HOST"
	onePasswordAccTokenEnv    = "P0_API_TOKEN"
	onePasswordAccDefaultHost = "https://api.p0.app"
)

// Skips TestAcc1Password unless it has a P0 organization to run against and every
// P0_1PASSWORD_* variable is set. It skips rather than fails, because CI sets TF_ACC
// without an organization.
func testAcc1PasswordPreCheck(t *testing.T) {
	if os.Getenv(onePasswordAccOrgEnv) == "" || os.Getenv(onePasswordAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the 1Password acceptance tests against a P0 organization", onePasswordAccOrgEnv, onePasswordAccTokenEnv)
	}
	var missing []string
	for name := range onePasswordAccEnv(&onePasswordAccInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the 1Password acceptance tests", strings.Join(missing, ", "))
	}
}

func onePasswordAccHost() string {
	if host := os.Getenv(onePasswordAccHostEnv); host != "" {
		return host
	}
	return onePasswordAccDefaultHost
}

// Calls the acceptance-test organization's API directly, to check that a destroyed
// install is gone.
func onePasswordAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        fmt.Sprintf("%s/o/%s", onePasswordAccHost(), os.Getenv(onePasswordAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(onePasswordAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// The chain the examples install, on AWS, for what the P0_1PASSWORD_* variables name.
// The provider reads the API token from P0_API_TOKEN itself.
func onePasswordAccConfig() string {
	var install onePasswordAccInstall
	for name, value := range onePasswordAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}

resource "p0_1password_connector_staged" "test" {
  id = %q
  hosting = {
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }
}

resource "p0_1password_connector" "test" {
  id      = p0_1password_connector_staged.test.id
  hosting = p0_1password_connector_staged.test.hosting
}

resource "p0_1password_account" "test" {
  id        = %q
  connector = p0_1password_connector.test.id
  secret_manager = {
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }
  token_secret_name = %q
}
`, os.Getenv(onePasswordAccOrgEnv), onePasswordAccHost(),
		install.connectorId, install.accountId, install.connectorName, install.connectorRegion,
		install.signInAddress, install.accountId, install.secretsRegion, install.tokenSecretName)
}

// Checks that P0 no longer has the item behind any destroyed 1Password resource.
func checkOnePasswordAccDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	components := map[string]string{
		"p0_1password_connector_staged": "connector",
		"p0_1password_connector":        "connector",
		"p0_1password_account":          "account",
	}
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := components[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(fmt.Sprintf("integrations/1password/config/%s/%s", component, rs.Primary.ID), &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the 1Password %s %s was deleted: %w", component, rs.Primary.ID, err)
			}
			return fmt.Errorf("the 1Password %s %s is still installed in P0", component, rs.Primary.ID)
		}
		return nil
	}
}

// TestAcc1Password installs a connector and an account in a P0 organization (see
// testAcc1PasswordPreCheck), checks that a second plan is empty, imports the account,
// and destroys both. The organization must run a P0 version with the 1Password
// integration.
func TestAcc1Password(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAcc1PasswordPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkOnePasswordAccDestroyed(onePasswordAccClient()),
		Steps: []resource.TestStep{
			{
				Config: onePasswordAccConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_1password_connector.test", "state", "installed"),
					resource.TestCheckResourceAttr("p0_1password_account.test", "state", "installed"),
					resource.TestCheckResourceAttrSet("p0_1password_account.test", "allowed_domains"),
				),
			},
			{
				ResourceName:      "p0_1password_account.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
