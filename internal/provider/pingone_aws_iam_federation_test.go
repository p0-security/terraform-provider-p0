package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	pingOneFederation    = "pingone-aws-iam-federation"
	pingOneConnector     = "prod"
	pingOneAppId         = "4f2a9c1e-8b3d-4e6f-a1c2-9d8e7f6a5b4c"
	pingOneEnvironmentId = "9c8b7a6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
	pingOneClientId      = "7d6c5b4a-3f2e-4d1c-9b8a-7f6e5d4c3b2a"
	pingOneMetadataUrl   = "https://auth.pingone.com/" + pingOneEnvironmentId + "/saml20/metadata/" + pingOneAppId
	pingOneConnectorName = "p0-pingone-prod"
)

// Where a connector runs, and the secret manager of the same cloud, as a customer's
// Terraform sets them.
type pingOneCloud struct {
	name    string
	hosting string
	vault   string
	secret  string
}

var pingOneClouds = []pingOneCloud{
	{
		name: "aws",
		hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-pingone-prod"
    connector_region = "us-west-2"
  }`,
		vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
		secret: "pingone/acme/p0",
	},
	{
		name: "gcp",
		hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-pingone-prod"
    connector_region = "us-west1"
  }`,
		vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
		secret: "pingone-acme-p0",
	},
}

// Behaves as P0's PingOne (AWS IAM Federation) installers do: verifying a Cloud Run
// connector resolves its URL, an app needs an installed connector, and the metadata names a connector's identity and an app's SAML
// metadata URL.
func newPingOneFederationFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if item["state"] == "stage" {
			return
		}
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = "https://p0-pingone-prod-abc123-uw.a.run.app"
		}
	}
	f.metadata = func(key fakeItemKey, item map[string]any) map[string]any {
		if key.component == "app" {
			return map[string]any{"samlMetadataUrl": pingOneMetadataUrl}
		}
		hosting, _ := item["hosting"].(map[string]any)
		if hosting["type"] == "gcp" {
			return map[string]any{"connectorIdentity": map[string]any{
				"type":                "gcp",
				"projectId":           hosting["projectId"],
				"serviceAccountEmail": fmt.Sprintf("%s@%s.iam.gserviceaccount.com", pingOneConnectorName, hosting["projectId"]),
				"serviceAccountId":    pingOneConnectorName,
			}}
		}
		return map[string]any{"connectorIdentity": map[string]any{
			"type":      "aws",
			"accountId": hosting["accountId"],
			"roleArn":   fmt.Sprintf("arn:aws:iam::%s:role/%s", hosting["accountId"], pingOneConnectorName),
			"roleName":  pingOneConnectorName,
		}}
	}
	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		switch {
		case step.nextState == "stage" && step.previous != nil:
			return http.StatusConflict, "The item is already added."
		case step.key.component == "app" && step.nextState == "stage":
			connector, _ := step.body["connector"].(string)
			if items[fakeItemKey{pingOneFederation, "connector", connector}]["state"] != "installed" {
				return http.StatusBadRequest, "Pick the connector that acts on this app."
			}
		}
		return 0, ""
	}
	return f
}

func pingOneFederationConfig(f *fakeP0, cloud pingOneCloud) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_pingone_aws_iam_federation_connector_staged" "example" {
  id      = %q
  hosting = %s
}

resource "p0_pingone_aws_iam_federation_connector" "example" {
  id      = p0_pingone_aws_iam_federation_connector_staged.example.id
  hosting = p0_pingone_aws_iam_federation_connector_staged.example.hosting
}

resource "p0_pingone_aws_iam_federation_app" "example" {
  id               = %q
  connector        = p0_pingone_aws_iam_federation_connector.example.id
  region           = "NA"
  environment_id   = %q
  role_attribute   = "awsRoles"
  client_id        = %q
  vault            = %s
  client_secret_name = %q
}
`, pingOneConnector, cloud.hosting, pingOneAppId, pingOneEnvironmentId, pingOneClientId, cloud.vault, cloud.secret)
}

// The full chain in each cloud. The framework plans again after each step and fails on
// a difference, so this also checks that what P0 assigns leaves none.
func TestPingOneAwsIamFederation(t *testing.T) {
	for _, cloud := range pingOneClouds {
		t.Run(cloud.name, func(t *testing.T) {
			f := newPingOneFederationFake(t)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					for _, key := range []fakeItemKey{{pingOneFederation, "connector", pingOneConnector}, {pingOneFederation, "app", pingOneAppId}} {
						if item := f.item(key.integration, key.component, key.id); item != nil {
							return fmt.Errorf("%s is still in P0: %v", key.component, item)
						}
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						Config: pingOneFederationConfig(f, cloud),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_pingone_aws_iam_federation_connector.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_pingone_aws_iam_federation_connector.example", "connector_identity.type", cloud.name),
							resource.TestCheckResourceAttr("p0_pingone_aws_iam_federation_app.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_pingone_aws_iam_federation_app.example", "saml_metadata_url", pingOneMetadataUrl),
							func(*terraform.State) error {
								item := f.item(pingOneFederation, "app", pingOneAppId)
								vault, _ := item["vault"].(map[string]any)
								if item["connector"] != pingOneConnector || item["environmentId"] != pingOneEnvironmentId || item["clientId"] != pingOneClientId ||
									item["clientSecretName"] != cloud.secret || vault["type"] != cloud.name+"-sm" {
									return fmt.Errorf("P0 item does not hold the configured app: %v", item)
								}
								return nil
							},
						),
					},
					{
						ResourceName:            "p0_pingone_aws_iam_federation_connector_staged.example",
						ImportState:             true,
						ImportStateId:           pingOneConnector,
						ImportStateVerify:       true,
						ImportStateVerifyIgnore: []string{"state"},
					},
					{
						ResourceName:      "p0_pingone_aws_iam_federation_connector.example",
						ImportState:       true,
						ImportStateId:     pingOneConnector,
						ImportStateVerify: true,
					},
					{
						ResourceName:      "p0_pingone_aws_iam_federation_app.example",
						ImportState:       true,
						ImportStateId:     pingOneAppId,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}
