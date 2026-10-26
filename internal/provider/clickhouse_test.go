package provider

// copy of internal/provider/auth0_test.go (#245), itself a copy of docker_test.go,
// renamed for ClickHouse Cloud (INT-695), with the four fields P0 reads when it
// validates an organization, and a key rotation or a new service that needs no
// apply.

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
	clickhouseConnector    = "prod"
	clickhouseOrganization = "4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f"
)

// The fields P0 reads from the connector when it validates an organization's install,
// as their attributes name them, and their JSON keys in P0's item.
var clickhouseValidatedFields = map[string]string{
	"organization_name": "organizationName",
	"api_key_id":        "apiKeyId",
	"allowed_domains":   "allowedDomains",
	"service_status":    "serviceStatus",
}

// What the connector reads when P0 first validates the organization.
func clickhouseFirstValidation() map[string]string {
	return map[string]string{
		"organizationName": "Acme",
		"apiKeyId":         "mFhGq3LrT9xKp2Wz8YbN",
		"allowedDomains":   `^example\.com$`,
		"serviceStatus":    "legacy: console-only (ClickHouse 25.8.1 is older than 26.4, so only console access can be requested)",
	}
}

// Where a ClickHouse Cloud connector runs, and the secret manager of the same cloud,
// as a customer's Terraform sets them.
type clickhouseCloud struct {
	hosting string
	vault   string
	secret  string
}

var clickhouseAws = clickhouseCloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-clickhouse-prod"
    connector_region = "us-west-2"
  }`,
	vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
	secret: "clickhouse/acme/api-key",
}

var clickhouseGcp = clickhouseCloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-clickhouse-prod"
    connector_region = "us-west1"
  }`,
	vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
	secret: "clickhouse-acme-api-key",
}

func clickhouseProviderConfig(f *clickhouseFake) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

func clickhouseStagedConfig(f *clickhouseFake, hosting string) string {
	return clickhouseProviderConfig(f) + fmt.Sprintf(`
resource "p0_clickhouse_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, clickhouseConnector, hosting)
}

// A connector staged and then installed, where the final resource takes the staged
// resource's hosting whole.
func clickhouseConnectorConfig(f *clickhouseFake, hosting string) string {
	return clickhouseStagedConfig(f, hosting) + `
resource "p0_clickhouse_connector" "example" {
  id      = p0_clickhouse_connector_staged.example.id
  hosting = p0_clickhouse_connector_staged.example.hosting
}
`
}

// A connector and an organization it acts on, as a customer's Terraform installs them.
func clickhouseConfig(f *clickhouseFake, cloud clickhouseCloud) string {
	return clickhouseConnectorConfig(f, cloud.hosting) + fmt.Sprintf(`
resource "p0_clickhouse_organization" "example" {
  id        = %q
  connector = p0_clickhouse_connector.example.id

  vault               = %s
  api_key_secret_name = %q
}
`, clickhouseOrganization, cloud.vault, cloud.secret)
}

// An in-memory stand-in for P0's install API for ClickHouse Cloud, behaving as P0's
// ClickHouse Cloud installers do:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - a PUT of an item that exists is refused, so an item can only be staged once
//   - an organization can only be staged for a connector that is installed
//   - verifying a connector on Cloud Run resolves its URL
//   - staging an organization stores "" for each field the connector reads, verifying
//     it stores what the connector reads, and configuring it keeps what's stored
//   - a connector's metadata names the identity it runs as
//   - unknown items are 404, and DELETE answers 204 with no body
//
// With failVerify set, verifying an organization fails as it does when the connector
// can't read the API key's secret.
type clickhouseFake struct {
	server *httptest.Server

	mu         sync.Mutex
	installed  bool
	items      map[clickhouseItemKey]map[string]any
	failVerify bool
	// What the connector reads when P0 validates the organization, by JSON key.
	validation map[string]string
}

// A ClickHouse Cloud item: a connector or an organization, by its ID.
type clickhouseItemKey struct {
	component, id string
}

func newClickHouseFake(t *testing.T) *clickhouseFake {
	f := &clickhouseFake{items: map[clickhouseItemKey]map[string]any{}, validation: clickhouseFirstValidation()}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/clickhouse/config"
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

func clickhouseItemKeyOf(r *http.Request) clickhouseItemKey {
	return clickhouseItemKey{r.PathValue("component"), r.PathValue("id")}
}

func (f *clickhouseFake) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.installed {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.installed = true
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *clickhouseFake) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		key := clickhouseItemKeyOf(r)
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
		f.normalize(key, updated)

		f.items[key] = updated
		writeClickHouseItem(w, key, updated)
	}
}

// Refuses a step the way P0's ClickHouse Cloud installers do: with an HTTP status and
// P0's message, or 0 to accept it. previous is the stored item, nil when there is
// none, and body is the request's. Call it with f.mu held.
func (f *clickhouseFake) refuse(key clickhouseItemKey, nextState string, previous, body map[string]any) (int, string) {
	switch {
	case nextState == "stage" && previous != nil:
		kind := key.component
		if kind == "organization" {
			kind = "ClickHouse Cloud organization"
		}
		return http.StatusConflict, fmt.Sprintf("The %s %s is already added. To change it, remove %s and add it again.", kind, key.id, key.id)
	case key.component == "organization" && nextState == "stage":
		connector, _ := body["connector"].(string)
		if f.items[clickhouseItemKey{"connector", connector}]["state"] != "installed" {
			return http.StatusBadRequest, "Pick the connector that acts on this organization. Install a ClickHouse Cloud connector first if there's none."
		}
	case key.component == "organization" && nextState == "configure" && f.failVerify:
		return http.StatusUnprocessableEntity, "The connector isn't allowed to read its ClickHouse Cloud API key secret in AWS Secrets Manager. " +
			"Grant the connector's identity secretsmanager:GetSecretValue on that secret, and kms:Decrypt on its KMS key if it uses a customer-managed key. " +
			"(P0 checked the API key's secret, and the connector's read grant on it.)"
	}
	return 0, ""
}

// Adds the fields that P0's ClickHouse Cloud installers add to an item at its new
// state. Call it with f.mu held.
func (f *clickhouseFake) normalize(key clickhouseItemKey, item map[string]any) {
	switch key.component {
	case "connector":
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" && item["state"] != "stage" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	case "organization":
		switch item["state"] {
		case "stage":
			for _, field := range clickhouseValidatedFields {
				item[field] = ""
			}
		// A verify stores what the connector reads, and configuring keeps it.
		case "configure":
			for field, value := range f.validation {
				item[field] = value
			}
		}
	}
}

// Computes an item's metadata, the identity a connector runs as. Nil omits the key, as
// P0 does.
func clickhouseItemMetadata(key clickhouseItemKey, item map[string]any) map[string]any {
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

func (f *clickhouseFake) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := clickhouseItemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	writeClickHouseItem(w, key, stored)
}

func (f *clickhouseFake) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := clickhouseItemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeClickHouseItem(w http.ResponseWriter, key clickhouseItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := clickhouseItemMetadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *clickhouseFake) item(component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[clickhouseItemKey{component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *clickhouseFake) update(component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[clickhouseItemKey{component, id}])
}

// Changes what the connector reads from then on, as rotating the API key or adding a
// service does.
func (f *clickhouseFake) changeValidation(field string, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.validation[field] = value
}

// Leaves the organization's install unfinished outside Terraform, as an item
// imported before its install finishes is.
func (f *clickhouseFake) leaveUnfinished() {
	f.update("organization", clickhouseOrganization, func(item map[string]any) {
		item["state"] = "configure"
	})
}

func (f *clickhouseFake) connector() map[string]any {
	return f.item("connector", clickhouseConnector)
}

func (f *clickhouseFake) organization() map[string]any {
	return f.item("organization", clickhouseOrganization)
}

func checkClickHouseDestroyed(f *clickhouseFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		if item := f.organization(); item != nil {
			return fmt.Errorf("the organization was not removed from P0: %v", item)
		}
		return nil
	}
}

// Checks that the organization's state holds each field P0 read from the connector, as
// validation has it.
func checkClickHouseValidated(validation map[string]string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{}
	for attribute, field := range clickhouseValidatedFields {
		checks = append(checks, resource.TestCheckResourceAttr("p0_clickhouse_organization.example", attribute, validation[field]))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

// The full chain on Google Cloud: the connector is staged once and then only verified
// and configured, since P0 refuses to stage an item twice.
func TestClickHouseOnGoogleCloud(t *testing.T) {
	f := newClickHouseFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClickHouseDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: clickhouseConfig(f, clickhouseGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.type", "gcp"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.service_account_id", "p0-clickhouse-prod"),
					resource.TestCheckResourceAttr(
						"p0_clickhouse_connector_staged.example", "connector_identity.service_account_email",
						"p0-clickhouse-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckNoResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.role_arn"),
					resource.TestCheckNoResourceAttr("p0_clickhouse_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "hosting.project_id", "acme-prod"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "hosting.connector_service_uri", "https://p0-clickhouse-prod-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(
						"p0_clickhouse_connector.example", "connector_identity.service_account_email",
						"p0-clickhouse-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					checkClickHouseValidated(clickhouseFirstValidation()),
					func(*terraform.State) error {
						item := f.organization()
						vault, _ := item["vault"].(map[string]any)
						if item["connector"] != clickhouseConnector || item["apiKeySecretName"] != clickhouseGcp.secret ||
							vault["type"] != "gcp-sm" || vault["install"] != "acme-secrets" || len(vault) != 2 {
							return fmt.Errorf("P0 item does not hold the configured organization: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_clickhouse_connector_staged.example",
				ImportState:       true,
				ImportStateId:     clickhouseConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      "p0_clickhouse_connector.example",
				ImportState:       true,
				ImportStateId:     clickhouseConnector,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_clickhouse_organization.example",
				ImportState:       true,
				ImportStateId:     clickhouseOrganization,
				ImportStateVerify: true,
			},
		},
	})
}

// The full chain on AWS, where P0 derives the connector's address, and a secret that
// has a region.
func TestClickHouseOnAws(t *testing.T) {
	f := newClickHouseFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClickHouseDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: clickhouseConfig(f, clickhouseAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.type", "aws"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.role_name", "p0-clickhouse-prod"),
					resource.TestCheckResourceAttr(
						"p0_clickhouse_connector_staged.example", "connector_identity.role_arn",
						"arn:aws:iam::123456789012:role/p0-clickhouse-prod",
					),
					resource.TestCheckNoResourceAttr("p0_clickhouse_connector_staged.example", "connector_identity.service_account_email"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_clickhouse_connector.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "vault.secrets_region", "us-west-2"),
					checkClickHouseValidated(clickhouseFirstValidation()),
					func(*terraform.State) error {
						item := f.organization()
						vault, _ := item["vault"].(map[string]any)
						if item["apiKeySecretName"] != clickhouseAws.secret || vault["type"] != "aws-sm" ||
							vault["install"] != "123456789012" || vault["secretsRegion"] != "us-west-2" {
							return fmt.Errorf("P0 item does not hold the configured organization: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_clickhouse_organization.example",
				ImportState:       true,
				ImportStateId:     clickhouseOrganization,
				ImportStateVerify: true,
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestClickHouseConnectorHostingReplacesBoth(t *testing.T) {
	f := newClickHouseFake(t)
	moved := `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-clickhouse-prod"
    connector_region = "europe-west1"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: clickhouseConnectorConfig(f, clickhouseGcp.hosting)},
			{
				Config: clickhouseConnectorConfig(f, moved),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_clickhouse_connector.example", plancheck.ResourceActionReplace),
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
func TestClickHouseConnectorRollback(t *testing.T) {
	f := newClickHouseFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClickHouseDestroyed(f),
		Steps: []resource.TestStep{
			{Config: clickhouseConnectorConfig(f, clickhouseGcp.hosting)},
			{
				Config: clickhouseStagedConfig(f, clickhouseGcp.hosting),
				Check: func(*terraform.State) error {
					item := f.connector()
					hosting, _ := item["hosting"].(map[string]any)
					if item == nil || item["state"] != "stage" || hosting["connectorName"] != "p0-clickhouse-prod" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: clickhouseConnectorConfig(f, clickhouseGcp.hosting),
				Check:  resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "state", "installed"),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestClickHouseUnfinishedInstallConverges(t *testing.T) {
	f := newClickHouseFake(t)
	restage := func(component, id string) {
		f.update(component, id, func(item map[string]any) {
			item["state"] = "stage"
			for _, field := range clickhouseValidatedFields {
				if _, ok := item[field]; ok {
					item[field] = ""
				}
			}
			if hosting, ok := item["hosting"].(map[string]any); ok {
				delete(hosting, "connectorServiceUri")
			}
		})
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: clickhouseConfig(f, clickhouseGcp)},
			{
				PreConfig: func() { restage("connector", clickhouseConnector) },
				Config:    clickhouseConfig(f, clickhouseGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_clickhouse_connector.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("p0_clickhouse_organization.example", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_clickhouse_connector.example", "hosting.connector_service_uri", "https://p0-clickhouse-prod-abc123-uw.a.run.app"),
				),
			},
			{
				PreConfig: func() { restage("organization", clickhouseOrganization) },
				Config:    clickhouseConfig(f, clickhouseGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_organization.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					checkClickHouseValidated(clickhouseFirstValidation()),
				),
			},
		},
	})
}

// Rotating the API key or adding a service needs no apply: P0 keeps what it stored at
// the install, so the next refresh plans no difference. An organization left
// unfinished outside Terraform plans an update, and applying it finishes the install
// with what the connector reads then.
func TestClickHouseRotatedKeyNeedsNoApply(t *testing.T) {
	f := newClickHouseFake(t)
	rotated := clickhouseFirstValidation()
	rotated["apiKeyId"] = "qT7vNw2XcK9pLm4RbZ3H"
	rotated["serviceStatus"] = ""
	withNewService := clickhouseFirstValidation()
	withNewService["apiKeyId"] = rotated["apiKeyId"]
	withNewService["serviceStatus"] = "analytics-byoc: not-ready (BYOC isn't supported)"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClickHouseDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: clickhouseConfig(f, clickhouseAws),
				Check:  checkClickHouseValidated(clickhouseFirstValidation()),
			},
			{
				// A new key, and the service that was too old upgraded.
				PreConfig: func() {
					for field, value := range rotated {
						f.changeValidation(field, value)
					}
				},
				Config: clickhouseConfig(f, clickhouseAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					checkClickHouseValidated(clickhouseFirstValidation()),
				),
			},
			{
				// A new service that isn't ready, and the organization left unfinished.
				PreConfig: func() {
					f.changeValidation("serviceStatus", withNewService["serviceStatus"])
					f.leaveUnfinished()
				},
				Config: clickhouseConfig(f, clickhouseAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_connector.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_clickhouse_organization.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					checkClickHouseValidated(withNewService),
					func(*terraform.State) error {
						if item := f.organization(); item["state"] != "installed" || item["serviceStatus"] != withNewService["serviceStatus"] {
							return fmt.Errorf("P0 item is not the organization installed again: %v", item)
						}
						return nil
					},
				),
			},
		},
	})
}

// An organization whose install check fails is left staged in P0 and tainted in
// Terraform. P0 refuses to stage it again, so the next apply replaces it: it removes
// the item, then stages and installs it.
func TestClickHouseOrganizationFailedCheckIsReplaced(t *testing.T) {
	f := newClickHouseFake(t)
	f.failVerify = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      clickhouseConfig(f, clickhouseAws),
				ExpectError: regexp.MustCompile(`(?s)ClickHouse Cloud organization install check failed.*isn't\s+allowed\s+to\s+read\s+its\s+ClickHouse\s+Cloud\s+API\s+key\s+secret`),
			},
			{
				PreConfig: func() {
					if item := f.organization(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failVerify = false
				},
				Config: clickhouseConfig(f, clickhouseAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_organization.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
					checkClickHouseValidated(clickhouseFirstValidation()),
				),
			},
		},
	})
}

// P0 adds an organization only for a connector that is installed. Naming one that is
// only staged fails the apply with P0's message and saves nothing, so the
// organization is created once its connector is installed.
func TestClickHouseOrganizationNeedsAnInstalledConnector(t *testing.T) {
	f := newClickHouseFake(t)
	organization := fmt.Sprintf(`
resource "p0_clickhouse_organization" "example" {
  id        = %q
  connector = p0_clickhouse_connector_staged.example.id

  vault               = %s
  api_key_secret_name = %q
}
`, clickhouseOrganization, clickhouseAws.vault, clickhouseAws.secret)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClickHouseDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      clickhouseStagedConfig(f, clickhouseAws.hosting) + organization,
				ExpectError: regexp.MustCompile(`(?s)Could not stage organization component.*Pick the connector`),
			},
			{
				PreConfig: func() {
					if item := f.organization(); item != nil {
						t.Errorf("P0 item after the refused organization = %v; want none", item)
					}
				},
				Config: clickhouseConfig(f, clickhouseAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_clickhouse_organization.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_clickhouse_organization.example", "state", "installed"),
			},
		},
	})
}

// An item that is already added in P0 can't be created, only imported.
func TestClickHouseRefusesAnItemThatExists(t *testing.T) {
	f := newClickHouseFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: clickhouseStagedConfig(f, clickhouseAws.hosting)},
			{
				Config: clickhouseStagedConfig(f, clickhouseAws.hosting) + fmt.Sprintf(`
resource "p0_clickhouse_connector_staged" "duplicate" {
  id      = %q
  hosting = %s
}
`, clickhouseConnector, clickhouseAws.hosting),
				ExpectError: regexp.MustCompile(`(?s)Could not stage connector component.*is already added`),
			},
		},
	})
}

func TestClickHouseValidation(t *testing.T) {
	awsHosting := func(field, value string) string {
		fields := map[string]string{"account_id": "123456789012", "connector_name": "p0-clickhouse-prod", "connector_region": "us-west-2"}
		fields[field] = value
		return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
	}
	organization := func(id, vault, secret string) string {
		return fmt.Sprintf(`
resource "p0_clickhouse_organization" "example" {
  id                  = %q
  connector           = "prod"
  vault               = %s
  api_key_secret_name = %q
}
`, id, vault, secret)
	}
	staged := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_clickhouse_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"connector id with capitals", staged("Prod", clickhouseAws.hosting), "lowercase letters"},
		{"connector name ECR wouldn't take", staged("prod", awsHosting("connector_name", "p0--clickhouse")), "Use 2 to 64 characters"},
		{"GovCloud connector", staged("prod", awsHosting("connector_region", "us-gov-west-1")), `P0's\s+ClickHouse\s+Cloud\s+integration\s+supports\s+only`},
		{
			"connector name too long for a service account",
			staged("prod", `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-clickhouse-connector-for-prod"
    connector_region = "us-west1"
  }`),
			"Use 6 to 30 characters",
		},
		{
			"project on AWS hosting",
			staged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-clickhouse-prod"
    connector_region = "us-west-2"
  }`),
			"Missing AWS account ID",
		},
		{"organization ID in capitals", organization("4F9B2C1E-7D3A-4E5B-9C8D-0A1B2C3D4E5F", clickhouseAws.vault, clickhouseAws.secret), `organization\s+ID,\s+a\s+UUID`},
		{"organization ID without hyphens", organization("4f9b2c1e7d3a4e5b9c8d0a1b2c3d4e5f", clickhouseAws.vault, clickhouseAws.secret), `organization\s+ID,\s+a\s+UUID`},
		{"organization name as its ID", organization("acme", clickhouseAws.vault, clickhouseAws.secret), `organization\s+ID,\s+a\s+UUID`},
		{"empty secret name", organization(clickhouseOrganization, clickhouseAws.vault, ""), `Enter\s+the\s+name\s+of\s+the\s+secret`},
		{"secret ARN", organization(clickhouseOrganization, clickhouseAws.vault, "arn:aws:secretsmanager:us-west-2:123456789012:secret:clickhouse/acme/api-key"), "doesn't look like a secret name"},
		{"AWS secret name in Google Secret Manager", organization(clickhouseOrganization, clickhouseGcp.vault, clickhouseAws.secret), "doesn't look like a secret name"},
		{
			"region on a Google vault",
			organization(clickhouseOrganization, `{
    type           = "gcp-sm"
    project_id     = "acme-secrets"
    secrets_region = "us-west-2"
  }`, clickhouseGcp.secret),
			"Unexpected secrets region",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newClickHouseFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      clickhouseProviderConfig(f) + tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}
