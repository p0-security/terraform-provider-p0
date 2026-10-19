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
	entraDirectory        = "00000000-0000-0000-0000-00000000000a"
	entraSubscription     = "00000000-0000-0000-0000-00000000000b"
	entraConnectorClient  = "00000000-0000-0000-0000-00000000000c"
	entraForgeClient      = "00000000-0000-0000-0000-00000000000d"
	entraAllowlist        = "Sites.Selected,User.Read.All"
	entraStagedAddress    = "p0_entra_agent_identity_creator_staged.example"
	entraFinalAddress     = "p0_entra_agent_identity_creator.example"
	entraServiceAccount   = "p0-sa@p0-prod.iam.gserviceaccount.com"
	entraServiceAccountId = "123456789012345678901"
	entraConnectorUri     = "https://p0-entra-agent-identity.example.eastus.azurecontainerapps.io"
)

var entraContainerApp = "/subscriptions/" + entraSubscription + "/resourceGroups/p0-entra-agent-identity/providers/Microsoft.App/containerApps/p0-entra-agent-identity"
var entraEnvironment = "/subscriptions/" + entraSubscription + "/resourceGroups/p0-entra-agent-identity/providers/Microsoft.App/managedEnvironments/p0-entra-agent-identity"

// The fields P0 accepts only when it creates the item (`step: "new"` in the app's
// packages/integrations/entra-agent-identity/shared/src/components.ts).
var entraNewStepFields = []string{"subscriptionId", "region", "appRoleAllowlist"}

func entraAgentIdentityStagedConfig(f *fakeP0) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_entra_agent_identity_creator_staged" "example" {
  directory_id       = %q
  subscription_id    = %q
  region             = "eastus"
  app_role_allowlist = %q
}
`, entraDirectory, entraSubscription, entraAllowlist)
}

// The staged and final resources, as a customer's Terraform installs them.
func entraAgentIdentityConfig(f *fakeP0, connectorClientId string) string {
	return entraAgentIdentityStagedConfig(f) + fmt.Sprintf(`
resource "p0_entra_agent_identity_creator" "example" {
  directory_id        = p0_entra_agent_identity_creator_staged.example.directory_id
  connector_client_id = %q
}
`, connectorClientId)
}

// Answers as the app's entra-agent-identity installer does: the assembler picks the
// service account and derives the connector's resource IDs when the item is staged,
// and the configurer reads the connector's URL and identity from its Container App.
// The fake also refuses a change to a staged-only field after staging, as P0 does, and
// records every write.
func newEntraAgentIdentityFake(t *testing.T) (*fakeP0, *[]fakeStep) {
	f := newFakeP0(t)
	steps := &[]fakeStep{}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "entra-agent-identity" || key.component != "iam-write" {
			return
		}
		if item["state"] == "stage" {
			subscription, _ := item["subscriptionId"].(string)
			group := "/subscriptions/" + subscription + "/resourceGroups/p0-entra-agent-identity/providers/Microsoft.App"
			item["serviceAccountEmail"] = entraServiceAccount
			item["serviceAccountId"] = entraServiceAccountId
			item["connectorName"] = "p0-entra-agent-identity"
			item["containerAppResourceId"] = group + "/containerApps/p0-entra-agent-identity"
			item["environmentId"] = group + "/managedEnvironments/p0-entra-agent-identity"
		}
		if item["state"] == "installed" {
			item["connectorServiceUri"] = entraConnectorUri
			item["forgeClientId"] = entraForgeClient
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		*steps = append(*steps, step)
		if step.nextState == "stage" {
			return 0, ""
		}
		for _, field := range entraNewStepFields {
			if value, ok := step.body[field]; ok && value != step.previous[field] {
				return http.StatusBadRequest, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
			}
		}
		return 0, ""
	}
	return f, steps
}

// The bodies of the recorded writes that moved an item to nextState.
func entraBodies(steps []fakeStep, nextState string) []map[string]any {
	bodies := []map[string]any{}
	for _, step := range steps {
		if step.nextState == nextState {
			bodies = append(bodies, step.body)
		}
	}
	return bodies
}

func TestEntraAgentIdentityCreator(t *testing.T) {
	f, steps := newEntraAgentIdentityFake(t)
	connectorBody := map[string]any{"connectorClientId": entraConnectorClient}
	newClient := "00000000-0000-0000-0000-0000000000ee"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if item := f.item("entra-agent-identity", "iam-write", entraDirectory); item != nil {
				return fmt.Errorf("the item was not removed from P0: %v", item)
			}
			return nil
		},
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 generates leave no diff.
			{
				Config: entraAgentIdentityConfig(f, entraConnectorClient),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraStagedAddress, "state", "stage"),
					resource.TestCheckResourceAttr(entraStagedAddress, "service_account_email", entraServiceAccount),
					resource.TestCheckResourceAttr(entraStagedAddress, "service_account_id", entraServiceAccountId),
					resource.TestCheckResourceAttr(entraFinalAddress, "state", "installed"),
					resource.TestCheckResourceAttr(entraFinalAddress, "connector_client_id", entraConnectorClient),
					resource.TestCheckResourceAttr(entraFinalAddress, "service_account_email", entraServiceAccount),
					resource.TestCheckResourceAttr(entraFinalAddress, "service_account_id", entraServiceAccountId),
					resource.TestCheckResourceAttr(entraFinalAddress, "container_app_resource_id", entraContainerApp),
					resource.TestCheckResourceAttr(entraFinalAddress, "environment_id", entraEnvironment),
					resource.TestCheckResourceAttr(entraFinalAddress, "connector_service_uri", entraConnectorUri),
					resource.TestCheckResourceAttr(entraFinalAddress, "forge_client_id", entraForgeClient),
					func(*terraform.State) error {
						item := f.item("entra-agent-identity", "iam-write", entraDirectory)
						if item["subscriptionId"] != entraSubscription || item["region"] != "eastus" || item["appRoleAllowlist"] != entraAllowlist {
							return fmt.Errorf("P0 item does not hold the staged inputs: %v", item)
						}
						// The staged-only inputs go out once, with the first stage. Every later
						// write carries the client ID alone.
						stages := entraBodies(*steps, "stage")
						want := map[string]any{"subscriptionId": entraSubscription, "region": "eastus", "appRoleAllowlist": entraAllowlist}
						if len(stages) != 2 || !reflect.DeepEqual(stages[0], want) || !reflect.DeepEqual(stages[1], connectorBody) {
							return fmt.Errorf("unexpected stage bodies: %v", stages)
						}
						for _, state := range []string{"configure", "installed"} {
							if bodies := entraBodies(*steps, state); len(bodies) != 1 || !reflect.DeepEqual(bodies[0], connectorBody) {
								return fmt.Errorf("unexpected bodies moving the item to %s: %v", state, bodies)
							}
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         entraStagedAddress,
				ImportState:                          true,
				ImportStateId:                        entraDirectory,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
				// The import reads the item as installed, while the staged resource's state
				// still holds "stage" from its own apply.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:                         entraFinalAddress,
				ImportState:                          true,
				ImportStateId:                        entraDirectory,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
			},
			// A new client ID updates the installed item with the configure step alone.
			{
				PreConfig: func() { *steps = nil },
				Config:    entraAgentIdentityConfig(f, newClient),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraFinalAddress, "connector_client_id", newClient),
					resource.TestCheckResourceAttr(entraFinalAddress, "state", "installed"),
					func(*terraform.State) error {
						only := *steps
						if len(only) != 1 || only[0].nextState != "installed" || !reflect.DeepEqual(only[0].body, map[string]any{"connectorClientId": newClient}) {
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
func TestEntraAgentIdentityCreatorRollback(t *testing.T) {
	f, _ := newEntraAgentIdentityFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: entraAgentIdentityConfig(f, entraConnectorClient),
			},
			{
				Config: entraAgentIdentityStagedConfig(f),
				Check: func(*terraform.State) error {
					item := f.item("entra-agent-identity", "iam-write", entraDirectory)
					if item == nil || item["state"] != "stage" || item["subscriptionId"] != entraSubscription {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// P0 offers the component only in development environments, and answers every write
// elsewhere with a bare 404.
func TestEntraAgentIdentityCreatorNotOffered(t *testing.T) {
	f, _ := newEntraAgentIdentityFake(t)
	f.refuse = func(fakeStep, map[fakeItemKey]map[string]any) (int, string) {
		return http.StatusNotFound, "Not found"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      entraAgentIdentityStagedConfig(f),
				ExpectError: regexp.MustCompile(`isn't available to this P0 organization`),
			},
		},
	})
}

func TestEntraAgentIdentityCreatorValidation(t *testing.T) {
	for _, tc := range []struct {
		name, directory, subscription, region, allowlist, connectorClient, error string
	}{
		{"uppercase directory", strings.ToUpper("0000000a-0000-0000-0000-00000000000a"), entraSubscription, "eastus", entraAllowlist, entraConnectorClient, "directory \\(tenant\\) ID as a lowercase GUID"},
		{"subscription not a guid", entraDirectory, "my-subscription", "eastus", entraAllowlist, entraConnectorClient, "subscription ID as a lowercase GUID"},
		{"region with a hyphen", entraDirectory, entraSubscription, "east-us", entraAllowlist, entraConnectorClient, "Azure region name"},
		{"empty allowlist", entraDirectory, entraSubscription, "eastus", "", entraConnectorClient, "app role values separated by commas"},
		{"trailing comma", entraDirectory, entraSubscription, "eastus", "Sites.Selected,", entraConnectorClient, "app role values separated by commas"},
		{"leading space", entraDirectory, entraSubscription, "eastus", " Sites.Selected", entraConnectorClient, "app role values separated by commas"},
		{"uppercase connector client", entraDirectory, entraSubscription, "eastus", entraAllowlist, strings.ToUpper("0000000c-0000-0000-0000-00000000000c"), "connector client ID as a lowercase GUID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newEntraAgentIdentityFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_entra_agent_identity_creator_staged" "example" {
  directory_id       = %q
  subscription_id    = %q
  region             = %q
  app_role_allowlist = %q
}

resource "p0_entra_agent_identity_creator" "example" {
  directory_id        = %q
  connector_client_id = %q
}
`, tc.directory, tc.subscription, tc.region, tc.allowlist, tc.directory, tc.connectorClient),
						PlanOnly: true,
						// Terraform wraps the message, so any whitespace matches a space.
						ExpectError: regexp.MustCompile(strings.ReplaceAll(tc.error, " ", `\s+`)),
					},
				},
			})
		})
	}
}

// What TestAccEntraAgentIdentityCreator installs. P0 checks the install against a real
// connector, so these name a directory where the connector's Terraform, rendered for
// the service account P0 picks for it, is applied.
var entraAgentIdentityAccEnv = []string{
	"P0_ENTRA_AGENT_IDENTITY_DIRECTORY_ID",
	"P0_ENTRA_AGENT_IDENTITY_SUBSCRIPTION_ID",
	"P0_ENTRA_AGENT_IDENTITY_REGION",
	"P0_ENTRA_AGENT_IDENTITY_APP_ROLE_ALLOWLIST",
	"P0_ENTRA_AGENT_IDENTITY_CONNECTOR_CLIENT_ID",
}

// Installs the Entra Agent Identity Creator into a real P0 organization, then imports
// it back. The organization must be in an environment that offers the component, and
// the connector must already be deployed and trust the service account P0 picks for
// the directory.
func TestAccEntraAgentIdentityCreator(t *testing.T) {
	missing := []string{}
	for _, name := range append([]string{"TF_ACC", "P0_ORG", "P0_API_TOKEN"}, entraAgentIdentityAccEnv...) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("Set %s to run the Entra Agent Identity Creator acceptance test", strings.Join(missing, ", "))
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

resource "p0_entra_agent_identity_creator_staged" "example" {
  directory_id       = %q
  subscription_id    = %q
  region             = %q
  app_role_allowlist = %q
}

resource "p0_entra_agent_identity_creator" "example" {
  directory_id        = p0_entra_agent_identity_creator_staged.example.directory_id
  connector_client_id = %q
}
`, os.Getenv("P0_ORG"), host,
		os.Getenv("P0_ENTRA_AGENT_IDENTITY_DIRECTORY_ID"),
		os.Getenv("P0_ENTRA_AGENT_IDENTITY_SUBSCRIPTION_ID"),
		os.Getenv("P0_ENTRA_AGENT_IDENTITY_REGION"),
		os.Getenv("P0_ENTRA_AGENT_IDENTITY_APP_ROLE_ALLOWLIST"),
		os.Getenv("P0_ENTRA_AGENT_IDENTITY_CONNECTOR_CLIENT_ID"))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraFinalAddress, "state", "installed"),
					resource.TestMatchResourceAttr(entraFinalAddress, "connector_service_uri", regexp.MustCompile(`^https://`)),
					resource.TestMatchResourceAttr(entraFinalAddress, "forge_client_id", regexp.MustCompile(`^[0-9a-f-]{36}$`)),
				),
			},
			{
				ResourceName:                         entraFinalAddress,
				ImportState:                          true,
				ImportStateId:                        os.Getenv("P0_ENTRA_AGENT_IDENTITY_DIRECTORY_ID"),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory_id",
			},
		},
	})
}
