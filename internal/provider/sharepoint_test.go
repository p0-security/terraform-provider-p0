package provider

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	sharePointConnectorClient = "00000000-0000-0000-0000-0000000000a1"
	sharePointStagedAddress   = "p0_sharepoint_connector_staged.example"
	sharePointFinalAddress    = "p0_sharepoint_connector.example"
	sharePointConnectorName   = "p0-sharepoint-ai"
	sharePointConnectorUri    = "https://p0-sharepoint-ai.example.eastus.azurecontainerapps.io"
)

var sharePointContainerApp = "/subscriptions/" + entraSubscription + "/resourceGroups/p0-entra-agent-identity/providers/Microsoft.App/containerApps/" + sharePointConnectorName

func sharePointStagedConfig(f *fakeP0) string {
	return entraAgentIdentityConfig(f, entraConnectorClient) + `
resource "p0_sharepoint_connector_staged" "example" {
  directory_id = p0_entra_agent_identity_creator.example.directory_id
}
`
}

// The Entra Agent Identity Creator and the SharePoint connector in one directory, as
// a customer's Terraform installs them.
func sharePointConfig(f *fakeP0, connectorClientId string) string {
	return sharePointStagedConfig(f) + fmt.Sprintf(`
resource "p0_sharepoint_connector" "example" {
  directory_id        = p0_sharepoint_connector_staged.example.directory_id
  connector_client_id = %q
}
`, connectorClientId)
}

// Extends the Entra Agent Identity Creator's fake with the app's sharepoint-ai
// installer: the assembler refuses a directory without an installed creator, then
// derives the connector's Container App in the creator's resource group, and the
// configurer reads the connector's URL. The item has no service account: P0 calls the
// connector as the creator's.
func newSharePointFake(t *testing.T) (*fakeP0, *[]fakeStep) {
	f, steps := newEntraAgentIdentityFake(t)
	forgeNormalize, forgeRefuse := f.normalize, f.refuse
	f.normalize = func(key fakeItemKey, item map[string]any) {
		forgeNormalize(key, item)
		if key.integration != "sharepoint-ai" || key.component != "connector" {
			return
		}
		if item["state"] == "stage" {
			item["connectorName"] = sharePointConnectorName
			item["containerAppResourceId"] = sharePointContainerApp
		}
		if item["state"] == "installed" {
			item["connectorServiceUri"] = sharePointConnectorUri
		}
	}
	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration == "sharepoint-ai" && step.nextState == "stage" {
			forge := items[fakeItemKey{"entra-agent-identity", "iam-write", step.key.id}]
			if forge == nil || forge["state"] != "installed" {
				*steps = append(*steps, step)
				return http.StatusUnprocessableEntity, fmt.Sprintf("P0 is not installed for 'Entra Agent Identity Creator' use 'iam-write' on '%s'. Please ask your P0 admin to install this integration component before continuing.", step.key.id)
			}
		}
		return forgeRefuse(step, items)
	}
	return f, steps
}

// The bodies of the recorded SharePoint writes that moved an item to nextState.
func sharePointBodies(steps []fakeStep, nextState string) []map[string]any {
	bodies := []map[string]any{}
	for _, step := range steps {
		if step.key.integration == "sharepoint-ai" && step.nextState == nextState {
			bodies = append(bodies, step.body)
		}
	}
	return bodies
}

func TestSharePointConnector(t *testing.T) {
	f, steps := newSharePointFake(t)
	connectorBody := map[string]any{"connectorClientId": sharePointConnectorClient}
	newClient := "00000000-0000-0000-0000-0000000000a2"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			for _, key := range [][2]string{{"entra-agent-identity", "iam-write"}, {"sharepoint-ai", "connector"}} {
				if item := f.item(key[0], key[1], entraDirectory); item != nil {
					return fmt.Errorf("the %s item was not removed from P0: %v", key[0], item)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 generates leave no diff.
			{
				Config: sharePointConfig(f, sharePointConnectorClient),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sharePointStagedAddress, "state", "stage"),
					resource.TestCheckResourceAttr(sharePointStagedAddress, "connector_name", sharePointConnectorName),
					resource.TestCheckResourceAttr(sharePointStagedAddress, "container_app_resource_id", sharePointContainerApp),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "state", "installed"),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "connector_client_id", sharePointConnectorClient),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "connector_name", sharePointConnectorName),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "container_app_resource_id", sharePointContainerApp),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "connector_service_uri", sharePointConnectorUri),
					func(*terraform.State) error {
						// The staged resource sends nothing, and every later write carries the
						// client ID alone.
						stages := sharePointBodies(*steps, "stage")
						if len(stages) != 2 || !reflect.DeepEqual(stages[0], map[string]any{}) || !reflect.DeepEqual(stages[1], connectorBody) {
							return fmt.Errorf("unexpected stage bodies: %v", stages)
						}
						for _, state := range []string{"configure", "installed"} {
							if bodies := sharePointBodies(*steps, state); len(bodies) != 1 || !reflect.DeepEqual(bodies[0], connectorBody) {
								return fmt.Errorf("unexpected bodies moving the item to %s: %v", state, bodies)
							}
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         sharePointStagedAddress,
				ImportState:                          true,
				ImportStateId:                        entraDirectory,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
				// The import reads the item as installed, while the staged resource's state
				// still holds "stage" from its own apply.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:                         sharePointFinalAddress,
				ImportState:                          true,
				ImportStateId:                        entraDirectory,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
			},
			// A new client ID updates the installed item with the configure step alone.
			{
				PreConfig: func() { *steps = nil },
				Config:    sharePointConfig(f, newClient),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sharePointFinalAddress, "connector_client_id", newClient),
					resource.TestCheckResourceAttr(sharePointFinalAddress, "state", "installed"),
					func(*terraform.State) error {
						only := *steps
						if len(only) != 1 || only[0].key.integration != "sharepoint-ai" || only[0].nextState != "installed" || !reflect.DeepEqual(only[0].body, map[string]any{"connectorClientId": newClient}) {
							return fmt.Errorf("expected one configure step with the new client ID, got %v", only)
						}
						return nil
					},
				),
			},
		},
	})
}

// Destroying only the final resource returns its item to "stage", so the staged
// resource still owns it.
func TestSharePointConnectorRollback(t *testing.T) {
	f, _ := newSharePointFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sharePointConfig(f, sharePointConnectorClient),
			},
			{
				Config: sharePointStagedConfig(f),
				Check: func(*terraform.State) error {
					item := f.item("sharepoint-ai", "connector", entraDirectory)
					if item == nil || item["state"] != "stage" || item["connectorName"] != sharePointConnectorName {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// P0 refuses to stage the connector for a directory without an installed Entra Agent
// Identity Creator, and the provider shows P0's message.
func TestSharePointConnectorWithoutCreator(t *testing.T) {
	f, _ := newSharePointFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_sharepoint_connector_staged" "example" {
  directory_id = %q
}
`, entraDirectory),
				// Terraform wraps the message, so any whitespace matches a space.
				ExpectError: regexp.MustCompile(`P0\s+is\s+not\s+installed\s+for\s+'Entra\s+Agent\s+Identity\s+Creator'`),
			},
		},
	})
}

// P0 offers the component only in development environments, and answers every write
// elsewhere with a bare 404.
func TestSharePointConnectorNotOffered(t *testing.T) {
	f, _ := newSharePointFake(t)
	f.refuse = func(fakeStep, map[fakeItemKey]map[string]any) (int, string) {
		return http.StatusNotFound, "Not found"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_sharepoint_connector_staged" "example" {
  directory_id = %q
}
`, entraDirectory),
				ExpectError: regexp.MustCompile(`SharePoint isn't available to this P0 organization`),
			},
		},
	})
}

func TestSharePointConnectorValidation(t *testing.T) {
	for _, tc := range []struct {
		name, directory, connectorClient, error string
	}{
		{"uppercase directory", strings.ToUpper("0000000a-0000-0000-0000-00000000000a"), sharePointConnectorClient, "directory \\(tenant\\) ID as a lowercase GUID"},
		{"directory not a guid", "contoso.onmicrosoft.com", sharePointConnectorClient, "directory \\(tenant\\) ID as a lowercase GUID"},
		{"uppercase connector client", entraDirectory, strings.ToUpper("000000a1-0000-0000-0000-0000000000a1"), "connector client ID as a lowercase GUID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newSharePointFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_sharepoint_connector_staged" "example" {
  directory_id = %q
}

resource "p0_sharepoint_connector" "example" {
  directory_id        = %q
  connector_client_id = %q
}
`, tc.directory, tc.directory, tc.connectorClient),
						PlanOnly: true,
						// Terraform wraps the message, so any whitespace matches a space.
						ExpectError: regexp.MustCompile(strings.ReplaceAll(tc.error, " ", `\s+`)),
					},
				},
			})
		})
	}
}

// What TestAccSharePointConnector installs. P0 checks the install against a real
// connector, so these name a directory where the Entra Agent Identity Creator is
// installed and the SharePoint connector's Terraform, trusting that creator's service
// account, is applied.
var sharePointAccEnv = []string{
	"P0_SHAREPOINT_DIRECTORY_ID",
	"P0_SHAREPOINT_CONNECTOR_CLIENT_ID",
}

// Installs the SharePoint connector into a real P0 organization, then imports it
// back. The organization must be in an environment that offers the component, the
// Entra Agent Identity Creator must be installed for the directory, and the connector
// must already be deployed and trust the creator's service account.
func TestAccSharePointConnector(t *testing.T) {
	missing := []string{}
	for _, name := range append([]string{"TF_ACC", "P0_ORG", "P0_API_TOKEN"}, sharePointAccEnv...) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("Set %s to run the SharePoint connector acceptance test", strings.Join(missing, ", "))
	}

	host := os.Getenv("P0_HOST")
	if host == "" {
		host = "https://api.p0.app"
	}
	config := fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}

resource "p0_sharepoint_connector_staged" "example" {
  directory_id = %q
}

resource "p0_sharepoint_connector" "example" {
  directory_id        = p0_sharepoint_connector_staged.example.directory_id
  connector_client_id = %q
}
`, os.Getenv("P0_ORG"), host,
		os.Getenv("P0_SHAREPOINT_DIRECTORY_ID"),
		os.Getenv("P0_SHAREPOINT_CONNECTOR_CLIENT_ID"))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sharePointFinalAddress, "state", "installed"),
					resource.TestMatchResourceAttr(sharePointFinalAddress, "connector_service_uri", regexp.MustCompile(`^https://`)),
				),
			},
			{
				ResourceName:                         sharePointFinalAddress,
				ImportState:                          true,
				ImportStateId:                        os.Getenv("P0_SHAREPOINT_DIRECTORY_ID"),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
			},
		},
	})
}
