package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	oktaFederation    = "okta-aws-iam-federation"
	oktaConnector     = "prod"
	oktaAppId         = "0oa1b2c3d4e5f6g7h8i9"
	oktaClientId      = "0oa9z8y7x6w5v4u3t2s1"
	oktaEntityId      = "http://www.okta.com/exk1b2c3d4e5f6g7h8i9"
	oktaMetadataUrl   = "https://acme.okta.com/app/exk1b2c3d4e5f6g7h8i9/sso/saml/metadata"
	oktaConnectorName = "p0-okta-prod"
)

// Where a connector runs, and the secret manager of the same cloud, as a customer's
// Terraform sets them.
type oktaCloud struct {
	name    string
	hosting string
	vault   string
	secret  string
}

var oktaClouds = []oktaCloud{
	{
		name: "aws",
		hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-okta-prod"
    connector_region = "us-west-2"
  }`,
		vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
		secret: "okta/acme/p0",
	},
	{
		name: "gcp",
		hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-okta-prod"
    connector_region = "us-west1"
  }`,
		vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
		secret: "okta-acme-p0",
	},
}

// Behaves as P0's Okta (AWS IAM Federation) installers do: verifying a Cloud Run
// connector resolves its URL, verifying an app reads its entity ID, an app needs an
// installed connector, and the metadata names a connector's identity and an app's SAML
// metadata URL.
func newOktaFederationFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if item["state"] == "stage" {
			return
		}
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = "https://p0-okta-prod-abc123-uw.a.run.app"
		}
		if key.component == "app" {
			item["entityId"] = oktaEntityId
		}
	}
	f.metadata = func(key fakeItemKey, item map[string]any) map[string]any {
		if key.component == "app" {
			if item["entityId"] == nil {
				return nil
			}
			return map[string]any{"samlMetadataUrl": oktaMetadataUrl}
		}
		hosting, _ := item["hosting"].(map[string]any)
		if hosting["type"] == "gcp" {
			return map[string]any{"connectorIdentity": map[string]any{
				"type":                "gcp",
				"projectId":           hosting["projectId"],
				"serviceAccountEmail": fmt.Sprintf("%s@%s.iam.gserviceaccount.com", oktaConnectorName, hosting["projectId"]),
				"serviceAccountId":    oktaConnectorName,
			}}
		}
		return map[string]any{"connectorIdentity": map[string]any{
			"type":      "aws",
			"accountId": hosting["accountId"],
			"roleArn":   fmt.Sprintf("arn:aws:iam::%s:role/%s", hosting["accountId"], oktaConnectorName),
			"roleName":  oktaConnectorName,
		}}
	}
	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		switch {
		case step.nextState == "stage" && step.previous != nil:
			return http.StatusConflict, "The item is already added."
		case step.key.component == "app" && step.nextState == "stage":
			connector, _ := step.body["connector"].(string)
			if items[fakeItemKey{oktaFederation, "connector", connector}]["state"] != "installed" {
				return http.StatusBadRequest, "Pick the connector that acts on this app."
			}
		}
		return 0, ""
	}
	return f
}

func oktaFederationConfig(f *fakeP0, cloud oktaCloud) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_okta_aws_iam_federation_connector_staged" "example" {
  id      = %q
  hosting = %s
}

resource "p0_okta_aws_iam_federation_connector" "example" {
  id      = p0_okta_aws_iam_federation_connector_staged.example.id
  hosting = p0_okta_aws_iam_federation_connector_staged.example.hosting
}

resource "p0_okta_aws_iam_federation_app" "example" {
  id               = %q
  connector        = p0_okta_aws_iam_federation_connector.example.id
  domain           = "acme.okta.com"
  client_id        = %q
  vault            = %s
  private_key_name = %q
}
`, oktaConnector, cloud.hosting, oktaAppId, oktaClientId, cloud.vault, cloud.secret)
}

// The full chain in each cloud. The framework plans again after each step and fails on
// a difference, so this also checks that what P0 assigns leaves none.
func TestOktaAwsIamFederation(t *testing.T) {
	for _, cloud := range oktaClouds {
		t.Run(cloud.name, func(t *testing.T) {
			f := newOktaFederationFake(t)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					for _, key := range []fakeItemKey{{oktaFederation, "connector", oktaConnector}, {oktaFederation, "app", oktaAppId}} {
						if item := f.item(key.integration, key.component, key.id); item != nil {
							return fmt.Errorf("%s is still in P0: %v", key.component, item)
						}
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						Config: oktaFederationConfig(f, cloud),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_okta_aws_iam_federation_connector.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_okta_aws_iam_federation_connector.example", "connector_identity.type", cloud.name),
							resource.TestCheckResourceAttr("p0_okta_aws_iam_federation_app.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_okta_aws_iam_federation_app.example", "entity_id", oktaEntityId),
							resource.TestCheckResourceAttr("p0_okta_aws_iam_federation_app.example", "saml_metadata_url", oktaMetadataUrl),
							func(*terraform.State) error {
								item := f.item(oktaFederation, "app", oktaAppId)
								vault, _ := item["vault"].(map[string]any)
								if item["connector"] != oktaConnector || item["domain"] != "acme.okta.com" || item["clientId"] != oktaClientId ||
									item["privateKeyName"] != cloud.secret || vault["type"] != cloud.name+"-sm" {
									return fmt.Errorf("P0 item does not hold the configured app: %v", item)
								}
								return nil
							},
						),
					},
					{
						ResourceName:            "p0_okta_aws_iam_federation_connector_staged.example",
						ImportState:             true,
						ImportStateId:           oktaConnector,
						ImportStateVerify:       true,
						ImportStateVerifyIgnore: []string{"state"},
					},
					{
						ResourceName:      "p0_okta_aws_iam_federation_connector.example",
						ImportState:       true,
						ImportStateId:     oktaConnector,
						ImportStateVerify: true,
					},
					{
						ResourceName:      "p0_okta_aws_iam_federation_app.example",
						ImportState:       true,
						ImportStateId:     oktaAppId,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}
