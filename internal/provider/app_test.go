package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	appId               = "internal-admin-tool"
	appDirectoryId      = "11111111-2222-3333-4444-555555555555"
	appSubscriptionId   = "66666666-7777-8888-9999-000000000000"
	appResourceGroup    = "p0-connectors"
	appContainerApp     = "p0-custom-app-connector"
	appServiceUri       = "https://p0-custom-app-connector.happyhill-1234abcd.eastus.azurecontainerapps.io"
	appConnectorClient  = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	appCloudRunUri      = "https://p0-custom-app-connector-abc123-uc.a.run.app"
	appResourceAddress  = "p0_app.example"
	appIntegrationKey   = "app"
	appComponentKey     = "iam-write"
	appAzureHostingType = "azure"
)

func appConfig(f *fakeP0, hosting string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_app" "example" {
  id      = %q
  hosting = %s
}
`, appId, hosting)
}

var appAzureHosting = fmt.Sprintf(`{
    type             = "azure"
    directory_id     = %q
    subscription_id  = %q
    resource_group   = %q
    connector_name   = %q
    connector_region = "eastus"
  }`, appDirectoryId, appSubscriptionId, appResourceGroup, appContainerApp)

// Sets the fields P0's verifier reads from the connector once it can reach it: the
// Cloud Run or Container App URL, and on Azure the app registration's client ID.
func newAppFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != appIntegrationKey || item["state"] == "stage" {
			return
		}
		hosting, _ := item["hosting"].(map[string]any)
		switch hosting["type"] {
		case "gcp":
			hosting["connectorServiceUri"] = appCloudRunUri
		case appAzureHostingType:
			hosting["connectorServiceUri"] = appServiceUri
			hosting["connectorClientId"] = appConnectorClient
		}
	}
	return f
}

func checkAppDestroyed(f *fakeP0) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.item(appIntegrationKey, appComponentKey, appId); item != nil {
			return fmt.Errorf("the custom application was not removed from P0: %v", item)
		}
		return nil
	}
}

func TestAppOnAzure(t *testing.T) {
	f := newAppFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAppDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 sets leave no diff.
			{
				Config: appConfig(f, appAzureHosting),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(appResourceAddress, "state", "installed"),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.type", appAzureHostingType),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.directory_id", appDirectoryId),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.subscription_id", appSubscriptionId),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.resource_group", appResourceGroup),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.connector_service_uri", appServiceUri),
					resource.TestCheckResourceAttr(appResourceAddress, "hosting.connector_client_id", appConnectorClient),
					resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.account_id"),
					resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.project_id"),
					func(*terraform.State) error {
						item := f.item(appIntegrationKey, appComponentKey, appId)
						hosting, _ := item["hosting"].(map[string]any)
						want := map[string]any{
							"type":            appAzureHostingType,
							"directoryId":     appDirectoryId,
							"subscriptionId":  appSubscriptionId,
							"resourceGroup":   appResourceGroup,
							"connectorName":   appContainerApp,
							"connectorRegion": "eastus",
						}
						for key, value := range want {
							if hosting[key] != value {
								return fmt.Errorf("P0 item's hosting.%s = %v; want %v (%v)", key, hosting[key], value, hosting)
							}
						}
						return nil
					},
				),
			},
			{
				ResourceName:      appResourceAddress,
				ImportState:       true,
				ImportStateId:     appId,
				ImportStateVerify: true,
			},
			// Every address field is `step: "new"` in P0, so a new resource group replaces
			// the install.
			{
				Config: appConfig(f, strings.Replace(appAzureHosting, appResourceGroup, "p0-connectors-eu", 1)),
				Check:  resource.TestCheckResourceAttr(appResourceAddress, "hosting.resource_group", "p0-connectors-eu"),
			},
		},
	})
}

// p0_app's provider sends only the address. P0 sets the URL and the client ID, so
// the provider never sends either back, even after reading them.
func TestAppOnAzureSendsOnlyTheAddress(t *testing.T) {
	f := newAppFake(t)
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		hosting, _ := step.body["hosting"].(map[string]any)
		for _, key := range []string{"connectorServiceUri", "connectorClientId"} {
			if _, ok := hosting[key]; ok {
				return 400, "the request sends " + key
			}
		}
		return 0, ""
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAppDestroyed(f),
		Steps: []resource.TestStep{
			{Config: appConfig(f, appAzureHosting)},
			// Replacing the install stages it again, now with the generated fields in
			// state.
			{Config: appConfig(f, strings.Replace(appAzureHosting, `"eastus"`, `"westeurope"`, 1))},
		},
	})
}

// The AWS and Google Cloud installs are as before: the Azure fields stay null, and
// re-planning after apply finds no diff.
func TestAppOnAwsAndGcp(t *testing.T) {
	cases := []struct {
		name    string
		hosting string
		checks  []resource.TestCheckFunc
	}{
		{
			name: "aws",
			hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-custom-app-connector"
    connector_region = "us-east-1"
  }`,
			checks: []resource.TestCheckFunc{
				resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.connector_service_uri"),
			},
		},
		{
			name: "gcp",
			hosting: `{
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-custom-app-connector"
    connector_region = "us-central1"
  }`,
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr(appResourceAddress, "hosting.connector_service_uri", appCloudRunUri),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAppFake(t)
			checks := append([]resource.TestCheckFunc{
				resource.TestCheckResourceAttr(appResourceAddress, "state", "installed"),
				resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.directory_id"),
				resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.subscription_id"),
				resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.resource_group"),
				resource.TestCheckNoResourceAttr(appResourceAddress, "hosting.connector_client_id"),
			}, c.checks...)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkAppDestroyed(f),
				Steps: []resource.TestStep{
					{
						Config: appConfig(f, c.hosting),
						Check:  resource.ComposeAggregateTestCheckFunc(checks...),
					},
					{
						ResourceName:      appResourceAddress,
						ImportState:       true,
						ImportStateId:     appId,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

// What TestAccAppOnAzure installs. P0 verifies the install against a real connector,
// so each of these names a Container App that is deployed and trusts P0.
var appAccAzureEnv = []string{
	"P0_APP_AZURE_DIRECTORY_ID",
	"P0_APP_AZURE_SUBSCRIPTION_ID",
	"P0_APP_AZURE_RESOURCE_GROUP",
	"P0_APP_AZURE_CONNECTOR_NAME",
	"P0_APP_AZURE_CONNECTOR_REGION",
}

// Installs an Azure-hosted custom application into a real P0 organization, then reads
// and imports it back. The organization's `azure` and `azure-app` installs must cover
// the connector's directory, and its environment must offer the `azure` option.
func TestAccAppOnAzure(t *testing.T) {
	missing := []string{}
	for _, name := range append([]string{"TF_ACC", "P0_ORG", "P0_API_TOKEN"}, appAccAzureEnv...) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("Set %s to run the Azure custom application acceptance test", strings.Join(missing, ", "))
	}

	host := os.Getenv("P0_HOST")
	if host == "" {
		host = "https://api.p0.app"
	}
	id := "tf-acc-azure-app"
	config := fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}

resource "p0_app" "example" {
  id = %q

  hosting = {
    type             = "azure"
    directory_id     = %q
    subscription_id  = %q
    resource_group   = %q
    connector_name   = %q
    connector_region = %q
  }
}
`, os.Getenv("P0_ORG"), host, id,
		os.Getenv("P0_APP_AZURE_DIRECTORY_ID"),
		os.Getenv("P0_APP_AZURE_SUBSCRIPTION_ID"),
		os.Getenv("P0_APP_AZURE_RESOURCE_GROUP"),
		os.Getenv("P0_APP_AZURE_CONNECTOR_NAME"),
		os.Getenv("P0_APP_AZURE_CONNECTOR_REGION"))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(appResourceAddress, "state", "installed"),
					resource.TestMatchResourceAttr(appResourceAddress, "hosting.connector_service_uri", regexp.MustCompile(`^https://`)),
					resource.TestMatchResourceAttr(appResourceAddress, "hosting.connector_client_id", regexp.MustCompile(`^[0-9a-f-]{36}$`)),
				),
			},
			{
				ResourceName:      appResourceAddress,
				ImportState:       true,
				ImportStateId:     id,
				ImportStateVerify: true,
			},
		},
	})
}
