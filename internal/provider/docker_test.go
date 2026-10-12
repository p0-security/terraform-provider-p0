package provider

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
	dockerConnector = "prod"
	dockerOrg       = "my-docker-org"
	dockerDomains   = `^example\.com$`
)

// Where a Docker Hub connector runs, and the secret manager of the same cloud, as a
// customer's Terraform sets them.
type dockerCloud struct {
	hosting string
	vault   string
	secret  string
}

var dockerAws = dockerCloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-docker-prod"
    connector_region = "us-west-2"
  }`,
	vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
	secret: "docker/my-docker-org/token",
}

var dockerGcp = dockerCloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-docker-prod"
    connector_region = "us-west1"
  }`,
	vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
	secret: "docker-my-docker-org-token",
}

func dockerProviderConfig(f *dockerFake) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

func dockerStagedConfig(f *dockerFake, hosting string) string {
	return dockerProviderConfig(f) + fmt.Sprintf(`
resource "p0_docker_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, dockerConnector, hosting)
}

// A connector staged and then installed, where the final resource takes the staged
// resource's hosting whole.
func dockerConnectorConfig(f *dockerFake, hosting string) string {
	return dockerStagedConfig(f, hosting) + `
resource "p0_docker_connector" "example" {
  id      = p0_docker_connector_staged.example.id
  hosting = p0_docker_connector_staged.example.hosting
}
`
}

// A connector and an organization it acts on, as a customer's Terraform installs them.
func dockerConfig(f *dockerFake, cloud dockerCloud) string {
	return dockerConnectorConfig(f, cloud.hosting) + fmt.Sprintf(`
resource "p0_docker_organization" "example" {
  id        = %q
  connector = p0_docker_connector.example.id

  vault             = %s
  token_secret_name = %q
}
`, dockerOrg, cloud.vault, cloud.secret)
}

// An in-memory stand-in for P0's install API for Docker Hub, behaving as P0's Docker
// Hub installers do:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - a PUT of an item that exists is refused, so an item can only be staged once
//   - an organization can only be staged for a connector that is installed
//   - verifying a connector on Cloud Run resolves its URL
//   - verifying an organization reads the allowed domains back from its connector
//   - a connector's metadata names the identity it runs as
//   - unknown items are 404, and DELETE answers 204 with no body
//
// With failVerify set, verifying an organization fails as it does when the connector
// can't read the token's secret.
type dockerFake struct {
	server *httptest.Server

	mu         sync.Mutex
	installed  bool
	items      map[dockerItemKey]map[string]any
	failVerify bool
}

// A Docker Hub item: a connector or an organization, by its ID.
type dockerItemKey struct {
	component, id string
}

func newDockerFake(t *testing.T) *dockerFake {
	f := &dockerFake{items: map[dockerItemKey]map[string]any{}}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/docker/config"
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

func dockerItemKeyOf(r *http.Request) dockerItemKey {
	return dockerItemKey{r.PathValue("component"), r.PathValue("id")}
}

func (f *dockerFake) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.installed {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.installed = true
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *dockerFake) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		key := dockerItemKeyOf(r)
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
		normalizeDockerItem(key, updated)

		f.items[key] = updated
		writeDockerItem(w, key, updated)
	}
}

// Refuses a step the way P0's Docker Hub installers do: with an HTTP status and P0's
// message, or 0 to accept it. previous is the stored item, nil when there is none, and
// body is the request's. Call it with f.mu held.
func (f *dockerFake) refuse(key dockerItemKey, nextState string, previous, body map[string]any) (int, string) {
	switch {
	case nextState == "stage" && previous != nil:
		return http.StatusConflict, fmt.Sprintf("The %s %s is already added. To change it, remove %s and add it again.", key.component, key.id, key.id)
	case key.component == "organization" && nextState == "stage":
		connector, _ := body["connector"].(string)
		if f.items[dockerItemKey{"connector", connector}]["state"] != "installed" {
			return http.StatusBadRequest, "Pick the connector that acts on this organization. Install a Docker Hub connector first if there's none."
		}
	case key.component == "organization" && nextState == "configure" && f.failVerify:
		return http.StatusUnprocessableEntity, "The connector isn't allowed to read the secret."
	}
	return 0, ""
}

// Adds the fields that P0's Docker Hub installers add when they verify an item.
func normalizeDockerItem(key dockerItemKey, item map[string]any) {
	if item["state"] == "stage" {
		return
	}
	switch key.component {
	case "connector":
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	case "organization":
		item["allowedDomains"] = dockerDomains
	}
}

// Computes an item's metadata, the identity a connector runs as. Nil omits the key, as
// P0 does.
func dockerItemMetadata(key dockerItemKey, item map[string]any) map[string]any {
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

func (f *dockerFake) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := dockerItemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	writeDockerItem(w, key, stored)
}

func (f *dockerFake) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := dockerItemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeDockerItem(w http.ResponseWriter, key dockerItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := dockerItemMetadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *dockerFake) item(component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[dockerItemKey{component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *dockerFake) update(component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[dockerItemKey{component, id}])
}

func (f *dockerFake) connector() map[string]any {
	return f.item("connector", dockerConnector)
}

func (f *dockerFake) organization() map[string]any {
	return f.item("organization", dockerOrg)
}

func checkDockerDestroyed(f *dockerFake) func(*terraform.State) error {
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

// The full chain on Google Cloud: the connector is staged once and then only verified
// and configured, since P0 refuses to stage an item twice.
func TestDockerOnGoogleCloud(t *testing.T) {
	f := newDockerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDockerDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: dockerConfig(f, dockerGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_docker_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_docker_connector_staged.example", "connector_identity.type", "gcp"),
					resource.TestCheckResourceAttr("p0_docker_connector_staged.example", "connector_identity.service_account_id", "p0-docker-prod"),
					resource.TestCheckResourceAttr(
						"p0_docker_connector_staged.example", "connector_identity.service_account_email",
						"p0-docker-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckNoResourceAttr("p0_docker_connector_staged.example", "connector_identity.role_arn"),
					resource.TestCheckNoResourceAttr("p0_docker_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "hosting.project_id", "acme-prod"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "hosting.connector_service_uri", "https://p0-docker-prod-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(
						"p0_docker_connector.example", "connector_identity.service_account_email",
						"p0-docker-prod@acme-prod.iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "allowed_domains", dockerDomains),
					func(*terraform.State) error {
						item := f.organization()
						vault, _ := item["vault"].(map[string]any)
						if item["connector"] != dockerConnector || item["tokenSecretName"] != dockerGcp.secret ||
							vault["type"] != "gcp-sm" || vault["install"] != "acme-secrets" || len(vault) != 2 {
							return fmt.Errorf("P0 item does not hold the configured organization: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_docker_connector_staged.example",
				ImportState:       true,
				ImportStateId:     dockerConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      "p0_docker_connector.example",
				ImportState:       true,
				ImportStateId:     dockerConnector,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_docker_organization.example",
				ImportState:       true,
				ImportStateId:     dockerOrg,
				ImportStateVerify: true,
			},
		},
	})
}

// The full chain on AWS, where P0 derives the connector's address, and a secret that
// has a region.
func TestDockerOnAws(t *testing.T) {
	f := newDockerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDockerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: dockerConfig(f, dockerAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_docker_connector_staged.example", "connector_identity.type", "aws"),
					resource.TestCheckResourceAttr("p0_docker_connector_staged.example", "connector_identity.role_name", "p0-docker-prod"),
					resource.TestCheckResourceAttr(
						"p0_docker_connector_staged.example", "connector_identity.role_arn",
						"arn:aws:iam::123456789012:role/p0-docker-prod",
					),
					resource.TestCheckNoResourceAttr("p0_docker_connector_staged.example", "connector_identity.service_account_email"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_docker_connector.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "vault.secrets_region", "us-west-2"),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "allowed_domains", dockerDomains),
					func(*terraform.State) error {
						item := f.organization()
						vault, _ := item["vault"].(map[string]any)
						if item["tokenSecretName"] != dockerAws.secret || vault["type"] != "aws-sm" ||
							vault["install"] != "123456789012" || vault["secretsRegion"] != "us-west-2" {
							return fmt.Errorf("P0 item does not hold the configured organization: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_docker_organization.example",
				ImportState:       true,
				ImportStateId:     dockerOrg,
				ImportStateVerify: true,
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestDockerConnectorHostingReplacesBoth(t *testing.T) {
	f := newDockerFake(t)
	moved := `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-docker-prod"
    connector_region = "europe-west1"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: dockerConnectorConfig(f, dockerGcp.hosting)},
			{
				Config: dockerConnectorConfig(f, moved),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_docker_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_docker_connector.example", plancheck.ResourceActionReplace),
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
func TestDockerConnectorRollback(t *testing.T) {
	f := newDockerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDockerDestroyed(f),
		Steps: []resource.TestStep{
			{Config: dockerConnectorConfig(f, dockerGcp.hosting)},
			{
				Config: dockerStagedConfig(f, dockerGcp.hosting),
				Check: func(*terraform.State) error {
					item := f.connector()
					hosting, _ := item["hosting"].(map[string]any)
					if item == nil || item["state"] != "stage" || hosting["connectorName"] != "p0-docker-prod" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: dockerConnectorConfig(f, dockerGcp.hosting),
				Check:  resource.TestCheckResourceAttr("p0_docker_connector.example", "state", "installed"),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestDockerUnfinishedInstallConverges(t *testing.T) {
	f := newDockerFake(t)
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
			{Config: dockerConfig(f, dockerGcp)},
			{
				PreConfig: func() { restage("connector", dockerConnector) },
				Config:    dockerConfig(f, dockerGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_docker_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_docker_connector.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("p0_docker_organization.example", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_docker_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_connector.example", "hosting.connector_service_uri", "https://p0-docker-prod-abc123-uw.a.run.app"),
				),
			},
			{
				PreConfig: func() { restage("organization", dockerOrg) },
				Config:    dockerConfig(f, dockerGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_docker_organization.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_docker_organization.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_docker_organization.example", "allowed_domains", dockerDomains),
				),
			},
		},
	})
}

// An organization whose install check fails is left staged in P0 and tainted in
// Terraform. P0 refuses to stage it again, so the next apply replaces it: it removes
// the item, then stages and installs it.
func TestDockerOrganizationFailedCheckIsReplaced(t *testing.T) {
	f := newDockerFake(t)
	f.failVerify = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      dockerConfig(f, dockerAws),
				ExpectError: regexp.MustCompile(`(?s)Docker Hub organization install check failed.*isn't allowed to read the\s+secret`),
			},
			{
				PreConfig: func() {
					if item := f.organization(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failVerify = false
				},
				Config: dockerConfig(f, dockerAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_docker_organization.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_docker_organization.example", "state", "installed"),
			},
		},
	})
}

// P0 adds an organization only for a connector that is installed. Naming one that is
// only staged fails the apply with P0's message and saves nothing, so the organization
// is created once its connector is installed.
func TestDockerOrganizationNeedsAnInstalledConnector(t *testing.T) {
	f := newDockerFake(t)
	organization := fmt.Sprintf(`
resource "p0_docker_organization" "example" {
  id        = %q
  connector = p0_docker_connector_staged.example.id

  vault             = %s
  token_secret_name = %q
}
`, dockerOrg, dockerAws.vault, dockerAws.secret)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDockerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      dockerStagedConfig(f, dockerAws.hosting) + organization,
				ExpectError: regexp.MustCompile(`(?s)Could not stage organization component.*Pick the connector`),
			},
			{
				PreConfig: func() {
					if item := f.organization(); item != nil {
						t.Errorf("P0 item after the refused organization = %v; want none", item)
					}
				},
				Config: dockerConfig(f, dockerAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_docker_organization.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_docker_organization.example", "state", "installed"),
			},
		},
	})
}

// An item that is already added in P0 can't be created, only imported.
func TestDockerRefusesAnItemThatExists(t *testing.T) {
	f := newDockerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: dockerStagedConfig(f, dockerAws.hosting)},
			{
				Config: dockerStagedConfig(f, dockerAws.hosting) + fmt.Sprintf(`
resource "p0_docker_connector_staged" "duplicate" {
  id      = %q
  hosting = %s
}
`, dockerConnector, dockerAws.hosting),
				ExpectError: regexp.MustCompile(`(?s)Could not stage connector component.*is already added`),
			},
		},
	})
}

func TestDockerValidation(t *testing.T) {
	awsHosting := func(field, value string) string {
		fields := map[string]string{"account_id": "123456789012", "connector_name": "p0-docker-prod", "connector_region": "us-west-2"}
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
resource "p0_docker_organization" "example" {
  id                = %q
  connector         = "prod"
  vault             = %s
  token_secret_name = %q
}
`, id, vault, secret)
	}
	staged := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_docker_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"connector id with capitals", staged("Prod", dockerAws.hosting), "lowercase letters"},
		{"connector name ECR wouldn't take", staged("prod", awsHosting("connector_name", "p0--docker")), "Use 2 to 64 characters"},
		{"GovCloud connector", staged("prod", awsHosting("connector_region", "us-gov-west-1")), "P0's Docker Hub integration supports only"},
		{
			"connector name too long for a service account",
			staged("prod", `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-docker-connector-for-production"
    connector_region = "us-west1"
  }`),
			"Use 6 to 30 characters",
		},
		{
			"project on AWS hosting",
			staged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-docker-prod"
    connector_region = "us-west-2"
  }`),
			"Missing AWS account ID",
		},
		{"organization name with capitals", organization("My-Docker-Org", dockerAws.vault, dockerAws.secret), "as Docker Hub shows it"},
		{"secret ARN", organization(dockerOrg, dockerAws.vault, "arn:aws:secretsmanager:us-west-2:123456789012:secret:token"), "doesn't look like a secret name"},
		{"AWS secret name in Google Secret Manager", organization(dockerOrg, dockerGcp.vault, dockerAws.secret), "doesn't look like a secret name"},
		{"access token in place of its secret's name", organization(dockerOrg, dockerAws.vault, "dckr_oat_example-Token_123"), "That's a Docker access token"},
		{
			"region on a Google vault",
			organization(dockerOrg, `{
    type           = "gcp-sm"
    project_id     = "acme-secrets"
    secrets_region = "us-west-2"
  }`, dockerGcp.secret),
			"Unexpected secrets region",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDockerFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      dockerProviderConfig(f) + tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}
