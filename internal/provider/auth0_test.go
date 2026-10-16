package provider

// copy of internal/provider/docker_test.go, renamed for Auth0 (INT-590), with the
// tenant's client ID.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	auth0Connector = "prod"
	auth0Tenant    = "acme.us.auth0.com"
	auth0ClientId  = "Q7z1pX9b2kWmN4rT6vY8cE0dFgHjLs3A"
	auth0Domains   = `^example\.com$`
)

// Where an Auth0 connector runs, and the secret manager of the same cloud, as a
// customer's Terraform sets them.
type auth0Cloud struct {
	hosting string
	vault   string
	secret  string
}

var auth0Aws = auth0Cloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-auth0-prod"
    connector_region = "us-west-2"
  }`,
	vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
	secret: "auth0/acme/mgmt",
}

var auth0Gcp = auth0Cloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-auth0-prod"
    connector_region = "us-west1"
  }`,
	vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
	secret: "auth0-acme-mgmt",
}

func auth0ProviderConfig(f *auth0Fake) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

func auth0StagedConfig(f *auth0Fake, hosting string) string {
	return auth0ProviderConfig(f) + fmt.Sprintf(`
resource "p0_auth0_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, auth0Connector, hosting)
}

// A connector staged and then installed, where the final resource takes the staged
// resource's hosting whole.
func auth0ConnectorConfig(f *auth0Fake, hosting string) string {
	return auth0StagedConfig(f, hosting) + `
resource "p0_auth0_connector" "example" {
  id      = p0_auth0_connector_staged.example.id
  hosting = p0_auth0_connector_staged.example.hosting
}
`
}

// A connector and a tenant it acts on, as a customer's Terraform installs them.
func auth0Config(f *auth0Fake, cloud auth0Cloud) string {
	return auth0ConnectorConfig(f, cloud.hosting) + fmt.Sprintf(`
resource "p0_auth0_tenant" "example" {
  id        = %q
  connector = p0_auth0_connector.example.id

  vault              = %s
  client_id          = %q
  client_secret_name = %q
}
`, auth0Tenant, cloud.vault, auth0ClientId, cloud.secret)
}

// An in-memory stand-in for P0's install API for Auth0, behaving as P0's Auth0
// installers do:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - a PUT of an item that exists is refused, so an item can only be staged once
//   - a tenant can only be staged for a connector that is installed
//   - verifying a connector on Cloud Run resolves its URL
//   - verifying a tenant reads the allowed domains back from its connector
//   - a connector's metadata names the identity it runs as
//   - unknown items are 404, and DELETE answers 204 with no body
//
// With failVerify set, verifying a tenant fails as it does when the connector
// can't read the client secret.
type auth0Fake struct {
	server *httptest.Server

	mu         sync.Mutex
	installed  bool
	items      map[auth0ItemKey]map[string]any
	failVerify bool
}

// An Auth0 item: a connector or a tenant, by its ID.
type auth0ItemKey struct {
	component, id string
}

func newAuth0Fake(t *testing.T) *auth0Fake {
	f := &auth0Fake{items: map[auth0ItemKey]map[string]any{}}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/auth0/config"
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

func auth0ItemKeyOf(r *http.Request) auth0ItemKey {
	return auth0ItemKey{r.PathValue("component"), r.PathValue("id")}
}

func (f *auth0Fake) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.installed {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.installed = true
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *auth0Fake) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		key := auth0ItemKeyOf(r)
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
		normalizeAuth0Item(key, updated)

		f.items[key] = updated
		writeAuth0Item(w, key, updated)
	}
}

// Refuses a step the way P0's Auth0 installers do: with an HTTP status and P0's
// message, or 0 to accept it. previous is the stored item, nil when there is none, and
// body is the request's. Call it with f.mu held.
func (f *auth0Fake) refuse(key auth0ItemKey, nextState string, previous, body map[string]any) (int, string) {
	switch {
	case nextState == "stage" && previous != nil:
		return http.StatusConflict, fmt.Sprintf("The %s %s is already added. To change it, remove %s and add it again.", key.component, key.id, key.id)
	case key.component == "tenant" && nextState == "stage":
		connector, _ := body["connector"].(string)
		if f.items[auth0ItemKey{"connector", connector}]["state"] != "installed" {
			return http.StatusBadRequest, "Pick the connector that acts on this tenant. Install an Auth0 connector first if there's none."
		}
	case key.component == "tenant" && nextState == "configure" && f.failVerify:
		return http.StatusUnprocessableEntity, "The connector isn't allowed to read the secret."
	}
	return 0, ""
}

// Adds the fields that P0's Auth0 installers add when they verify an item.
func normalizeAuth0Item(key auth0ItemKey, item map[string]any) {
	if item["state"] == "stage" {
		return
	}
	switch key.component {
	case "connector":
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	case "tenant":
		item["allowedDomains"] = auth0Domains
	}
}

// Computes an item's metadata, the identity a connector runs as. Nil omits the key, as
// P0 does.
func auth0ItemMetadata(key auth0ItemKey, item map[string]any) map[string]any {
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

func (f *auth0Fake) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := auth0ItemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	writeAuth0Item(w, key, stored)
}

func (f *auth0Fake) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := auth0ItemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeAuth0Item(w http.ResponseWriter, key auth0ItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := auth0ItemMetadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *auth0Fake) item(component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[auth0ItemKey{component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *auth0Fake) update(component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[auth0ItemKey{component, id}])
}

func (f *auth0Fake) connector() map[string]any {
	return f.item("connector", auth0Connector)
}

func (f *auth0Fake) tenant() map[string]any {
	return f.item("tenant", auth0Tenant)
}

func checkAuth0Destroyed(f *auth0Fake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		if item := f.tenant(); item != nil {
			return fmt.Errorf("the tenant was not removed from P0: %v", item)
		}
		return nil
	}
}

// The full chain on Google Cloud: the connector is staged once and then only verified
// and configured, since P0 refuses to stage an item twice.
func TestAuth0OnGoogleCloud(t *testing.T) {
	f := newAuth0Fake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAuth0Destroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: auth0Config(f, auth0Gcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_auth0_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_auth0_connector_staged.example", "connector_identity.type", "gcp"),
					resource.TestCheckResourceAttr("p0_auth0_connector_staged.example", "connector_identity.service_account_id", "p0-auth0-prod"),
					resource.TestCheckResourceAttr(
						"p0_auth0_connector_staged.example", "connector_identity.service_account_email",
						"p0-auth0-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckNoResourceAttr("p0_auth0_connector_staged.example", "connector_identity.role_arn"),
					resource.TestCheckNoResourceAttr("p0_auth0_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "hosting.project_id", "acme-prod"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "hosting.connector_service_uri", "https://p0-auth0-prod-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(
						"p0_auth0_connector.example", "connector_identity.service_account_email",
						"p0-auth0-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "allowed_domains", auth0Domains),
					func(*terraform.State) error {
						item := f.tenant()
						vault, _ := item["vault"].(map[string]any)
						if item["connector"] != auth0Connector || item["clientId"] != auth0ClientId || item["clientSecretName"] != auth0Gcp.secret ||
							vault["type"] != "gcp-sm" || vault["install"] != "acme-secrets" || len(vault) != 2 {
							return fmt.Errorf("P0 item does not hold the configured tenant: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_auth0_connector_staged.example",
				ImportState:       true,
				ImportStateId:     auth0Connector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      "p0_auth0_connector.example",
				ImportState:       true,
				ImportStateId:     auth0Connector,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_auth0_tenant.example",
				ImportState:       true,
				ImportStateId:     auth0Tenant,
				ImportStateVerify: true,
			},
		},
	})
}

// The full chain on AWS, where P0 derives the connector's address, and a secret that
// has a region.
func TestAuth0OnAws(t *testing.T) {
	f := newAuth0Fake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAuth0Destroyed(f),
		Steps: []resource.TestStep{
			{
				Config: auth0Config(f, auth0Aws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_auth0_connector_staged.example", "connector_identity.type", "aws"),
					resource.TestCheckResourceAttr("p0_auth0_connector_staged.example", "connector_identity.role_name", "p0-auth0-prod"),
					resource.TestCheckResourceAttr(
						"p0_auth0_connector_staged.example", "connector_identity.role_arn",
						"arn:aws:iam::123456789012:role/p0-auth0-prod",
					),
					resource.TestCheckNoResourceAttr("p0_auth0_connector_staged.example", "connector_identity.service_account_email"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_auth0_connector.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "vault.secrets_region", "us-west-2"),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "allowed_domains", auth0Domains),
					func(*terraform.State) error {
						item := f.tenant()
						vault, _ := item["vault"].(map[string]any)
						if item["clientId"] != auth0ClientId || item["clientSecretName"] != auth0Aws.secret || vault["type"] != "aws-sm" ||
							vault["install"] != "123456789012" || vault["secretsRegion"] != "us-west-2" {
							return fmt.Errorf("P0 item does not hold the configured tenant: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_auth0_tenant.example",
				ImportState:       true,
				ImportStateId:     auth0Tenant,
				ImportStateVerify: true,
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestAuth0ConnectorHostingReplacesBoth(t *testing.T) {
	f := newAuth0Fake(t)
	moved := `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-auth0-prod"
    connector_region = "europe-west1"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: auth0ConnectorConfig(f, auth0Gcp.hosting)},
			{
				Config: auth0ConnectorConfig(f, moved),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_auth0_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_auth0_connector.example", plancheck.ResourceActionReplace),
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
func TestAuth0ConnectorRollback(t *testing.T) {
	f := newAuth0Fake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAuth0Destroyed(f),
		Steps: []resource.TestStep{
			{Config: auth0ConnectorConfig(f, auth0Gcp.hosting)},
			{
				Config: auth0StagedConfig(f, auth0Gcp.hosting),
				Check: func(*terraform.State) error {
					item := f.connector()
					hosting, _ := item["hosting"].(map[string]any)
					if item == nil || item["state"] != "stage" || hosting["connectorName"] != "p0-auth0-prod" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: auth0ConnectorConfig(f, auth0Gcp.hosting),
				Check:  resource.TestCheckResourceAttr("p0_auth0_connector.example", "state", "installed"),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestAuth0UnfinishedInstallConverges(t *testing.T) {
	f := newAuth0Fake(t)
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
			{Config: auth0Config(f, auth0Gcp)},
			{
				PreConfig: func() { restage("connector", auth0Connector) },
				Config:    auth0Config(f, auth0Gcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_auth0_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_auth0_connector.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("p0_auth0_tenant.example", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_connector.example", "hosting.connector_service_uri", "https://p0-auth0-prod-abc123-uw.a.run.app"),
				),
			},
			{
				PreConfig: func() { restage("tenant", auth0Tenant) },
				Config:    auth0Config(f, auth0Gcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_auth0_tenant.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_auth0_tenant.example", "allowed_domains", auth0Domains),
				),
			},
		},
	})
}

// A tenant whose install check fails is left staged in P0 and tainted in
// Terraform. P0 refuses to stage it again, so the next apply replaces it: it removes
// the item, then stages and installs it.
func TestAuth0TenantFailedCheckIsReplaced(t *testing.T) {
	f := newAuth0Fake(t)
	f.failVerify = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      auth0Config(f, auth0Aws),
				ExpectError: regexp.MustCompile(`(?s)Auth0 tenant install check failed.*isn't allowed to read the\s+secret`),
			},
			{
				PreConfig: func() {
					if item := f.tenant(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failVerify = false
				},
				Config: auth0Config(f, auth0Aws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_auth0_tenant.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_auth0_tenant.example", "state", "installed"),
			},
		},
	})
}

// P0 adds a tenant only for a connector that is installed. Naming one that is
// only staged fails the apply with P0's message and saves nothing, so the tenant
// is created once its connector is installed.
func TestAuth0TenantNeedsAnInstalledConnector(t *testing.T) {
	f := newAuth0Fake(t)
	tenant := fmt.Sprintf(`
resource "p0_auth0_tenant" "example" {
  id        = %q
  connector = p0_auth0_connector_staged.example.id

  vault              = %s
  client_id          = %q
  client_secret_name = %q
}
`, auth0Tenant, auth0Aws.vault, auth0ClientId, auth0Aws.secret)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAuth0Destroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      auth0StagedConfig(f, auth0Aws.hosting) + tenant,
				ExpectError: regexp.MustCompile(`(?s)Could not stage tenant component.*Pick the connector`),
			},
			{
				PreConfig: func() {
					if item := f.tenant(); item != nil {
						t.Errorf("P0 item after the refused tenant = %v; want none", item)
					}
				},
				Config: auth0Config(f, auth0Aws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_auth0_tenant.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_auth0_tenant.example", "state", "installed"),
			},
		},
	})
}

// An item that is already added in P0 can't be created, only imported.
func TestAuth0RefusesAnItemThatExists(t *testing.T) {
	f := newAuth0Fake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: auth0StagedConfig(f, auth0Aws.hosting)},
			{
				Config: auth0StagedConfig(f, auth0Aws.hosting) + fmt.Sprintf(`
resource "p0_auth0_connector_staged" "duplicate" {
  id      = %q
  hosting = %s
}
`, auth0Connector, auth0Aws.hosting),
				ExpectError: regexp.MustCompile(`(?s)Could not stage connector component.*is already added`),
			},
		},
	})
}

func TestAuth0Validation(t *testing.T) {
	awsHosting := func(field, value string) string {
		fields := map[string]string{"account_id": "123456789012", "connector_name": "p0-auth0-prod", "connector_region": "us-west-2"}
		fields[field] = value
		return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
	}
	tenantWithClient := func(id, vault, clientId, secret string) string {
		return fmt.Sprintf(`
resource "p0_auth0_tenant" "example" {
  id                 = %q
  connector          = "prod"
  vault              = %s
  client_id          = %q
  client_secret_name = %q
}
`, id, vault, clientId, secret)
	}
	tenant := func(id, vault, secret string) string {
		return tenantWithClient(id, vault, auth0ClientId, secret)
	}
	staged := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_auth0_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"connector id with capitals", staged("Prod", auth0Aws.hosting), "lowercase letters"},
		{"connector name ECR wouldn't take", staged("prod", awsHosting("connector_name", "p0--auth0")), "Use 2 to 64 characters"},
		{"GovCloud connector", staged("prod", awsHosting("connector_region", "us-gov-west-1")), "P0's Auth0 integration supports only"},
		{
			"connector name too long for a service account",
			staged("prod", `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-auth0-connector-for-production"
    connector_region = "us-west1"
  }`),
			"Use 6 to 30 characters",
		},
		{
			"project on AWS hosting",
			staged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-auth0-prod"
    connector_region = "us-west-2"
  }`),
			"Missing AWS account ID",
		},
		{"custom domain", tenant("login.acme.com", auth0Aws.vault, auth0Aws.secret), `canonical\s+Auth0\s+domain`},
		{"tenant domain with capitals", tenant("Acme.us.auth0.com", auth0Aws.vault, auth0Aws.secret), `canonical\s+Auth0\s+domain`},
		{"client ID with a space", tenantWithClient(auth0Tenant, auth0Aws.vault, "abc def", auth0Aws.secret), `Enter\s+the\s+client\s+ID`},
		{"empty client ID", tenantWithClient(auth0Tenant, auth0Aws.vault, "", auth0Aws.secret), `Enter\s+the\s+client\s+ID`},
		{"secret ARN", tenant(auth0Tenant, auth0Aws.vault, "arn:aws:secretsmanager:us-west-2:123456789012:secret:auth0/acme/mgmt"), "doesn't look like a secret name"},
		{"AWS secret name in Google Secret Manager", tenant(auth0Tenant, auth0Gcp.vault, auth0Aws.secret), "doesn't look like a secret name"},
		{
			"region on a Google vault",
			tenant(auth0Tenant, `{
    type           = "gcp-sm"
    project_id     = "acme-secrets"
    secrets_region = "us-west-2"
  }`, auth0Gcp.secret),
			"Unexpected secrets region",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuth0Fake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      auth0ProviderConfig(f) + tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}
