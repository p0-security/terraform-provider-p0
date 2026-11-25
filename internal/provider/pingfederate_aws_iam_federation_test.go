package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// copy of internal/provider/pingone_aws_iam_federation_test.go, renamed for PingFederate
// (AWS IAM Federation) (INT-1565), with PingFederate's URL and SCIM user name in place
// of PingFederate's IDs and region.

const (
	pingFederateFederation    = "pingfederate-aws-iam-federation"
	pingFederateConnector     = "prod"
	pingFederateAppId         = "acme-aws"
	pingFederateUrl           = "https://sso.example.com:9031"
	pingFederateScimUsername  = "p0"
	pingFederateMetadataUrl   = pingFederateUrl + "/pf/federation_metadata.ping?PartnerSpId=urn:amazon:webservices"
	pingFederateConnectorName = "p0-pingfederate-prod"
	pingFederateAudience      = "https://api.example/integrations/pingfederate-aws-iam-federation/tenants/t1/apps/" + pingFederateAppId
)

// Where a connector runs, and the secret manager of the same cloud, as a customer's
// Terraform sets them.
type pingFederateCloud struct {
	name    string
	hosting string
	vault   string
	secret  string
}

var pingFederateClouds = []pingFederateCloud{
	{
		name: "aws",
		hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-pingfederate-prod"
    connector_region = "us-west-2"
  }`,
		vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
		secret: "pingfederate/acme/p0",
	},
	{
		name: "gcp",
		hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-pingfederate-prod"
    connector_region = "us-west1"
  }`,
		vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
		secret: "pingfederate-acme-p0",
	},
}

// Behaves as P0's PingFederate (AWS IAM Federation) installers do: verifying a Cloud Run
// connector resolves its URL, an app needs an installed connector, and the metadata names a connector's identity and an app's SAML
// metadata URL.
func newPingFederateFederationFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if source, ok := item["roleSource"].(map[string]any); ok && source["type"] == "attribute-source" {
			source["audience"] = pingFederateAudience
		}
		if item["state"] == "stage" {
			return
		}
		if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
			hosting["connectorServiceUri"] = "https://p0-pingfederate-prod-abc123-uw.a.run.app"
		}
	}
	f.metadata = func(key fakeItemKey, item map[string]any) map[string]any {
		if key.component == "app" {
			return map[string]any{"samlMetadataUrl": pingFederateMetadataUrl}
		}
		hosting, _ := item["hosting"].(map[string]any)
		if hosting["type"] == "gcp" {
			return map[string]any{"connectorIdentity": map[string]any{
				"type":                "gcp",
				"projectId":           hosting["projectId"],
				"serviceAccountEmail": fmt.Sprintf("%s@%s.iam.gserviceaccount.com", pingFederateConnectorName, hosting["projectId"]),
				"serviceAccountId":    pingFederateConnectorName,
			}}
		}
		return map[string]any{"connectorIdentity": map[string]any{
			"type":      "aws",
			"accountId": hosting["accountId"],
			"roleArn":   fmt.Sprintf("arn:aws:iam::%s:role/%s", hosting["accountId"], pingFederateConnectorName),
			"roleName":  pingFederateConnectorName,
		}}
	}
	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		switch {
		case step.nextState == "stage" && step.previous != nil:
			return http.StatusConflict, "The item is already added."
		case step.key.component == "app" && step.nextState == "stage":
			source, _ := step.body["roleSource"].(map[string]any)
			if source["type"] != "scim" {
				break
			}
			connector, _ := source["connector"].(string)
			if items[fakeItemKey{pingFederateFederation, "connector", connector}]["state"] != "installed" {
				return http.StatusBadRequest, "Pick the connector that acts on this app."
			}
		}
		return 0, ""
	}
	return f
}

func pingFederateFederationConfig(f *fakeP0, cloud pingFederateCloud) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_pingfederate_aws_iam_federation_connector_staged" "example" {
  id      = %q
  hosting = %s
}

resource "p0_pingfederate_aws_iam_federation_connector" "example" {
  id      = p0_pingfederate_aws_iam_federation_connector_staged.example.id
  hosting = p0_pingfederate_aws_iam_federation_connector_staged.example.hosting
}

resource "p0_pingfederate_aws_iam_federation_app" "example" {
  id               = %q
  pingfederate_url = %q
  scim = {
    connector          = p0_pingfederate_aws_iam_federation_connector.example.id
    role_attribute     = "awsRole"
    scim_username      = %q
    vault              = %s
    scim_password_name = %q
  }
}
`, pingFederateConnector, cloud.hosting, pingFederateAppId, pingFederateUrl, pingFederateScimUsername, cloud.vault, cloud.secret)
}

// The full chain in each cloud. The framework plans again after each step and fails on
// a difference, so this also checks that what P0 assigns leaves none.
func TestPingFederateAwsIamFederation(t *testing.T) {
	for _, cloud := range pingFederateClouds {
		t.Run(cloud.name, func(t *testing.T) {
			f := newPingFederateFederationFake(t)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					for _, key := range []fakeItemKey{{pingFederateFederation, "connector", pingFederateConnector}, {pingFederateFederation, "app", pingFederateAppId}} {
						if item := f.item(key.integration, key.component, key.id); item != nil {
							return fmt.Errorf("%s is still in P0: %v", key.component, item)
						}
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						Config: pingFederateFederationConfig(f, cloud),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_pingfederate_aws_iam_federation_connector.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_pingfederate_aws_iam_federation_connector.example", "connector_identity.type", cloud.name),
							resource.TestCheckResourceAttr("p0_pingfederate_aws_iam_federation_app.example", "state", "installed"),
							resource.TestCheckResourceAttr("p0_pingfederate_aws_iam_federation_app.example", "saml_metadata_url", pingFederateMetadataUrl),
							func(*terraform.State) error {
								item := f.item(pingFederateFederation, "app", pingFederateAppId)
								source, _ := item["roleSource"].(map[string]any)
								vault, _ := source["vault"].(map[string]any)
								if source["type"] != "scim" || source["connector"] != pingFederateConnector || item["pingFederateUrl"] != pingFederateUrl || source["scimUsername"] != pingFederateScimUsername ||
									source["scimPasswordName"] != cloud.secret || vault["type"] != cloud.name+"-sm" {
									return fmt.Errorf("P0 item does not hold the configured app: %v", item)
								}
								return nil
							},
						),
					},
					{
						ResourceName:            "p0_pingfederate_aws_iam_federation_connector_staged.example",
						ImportState:             true,
						ImportStateId:           pingFederateConnector,
						ImportStateVerify:       true,
						ImportStateVerifyIgnore: []string{"state"},
					},
					{
						ResourceName:      "p0_pingfederate_aws_iam_federation_connector.example",
						ImportState:       true,
						ImportStateId:     pingFederateConnector,
						ImportStateVerify: true,
					},
					{
						ResourceName:      "p0_pingfederate_aws_iam_federation_app.example",
						ImportState:       true,
						ImportStateId:     pingFederateAppId,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

func pingFederateAttributeSourceConfig(f *fakeP0, issuer string, jwks string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_pingfederate_aws_iam_federation_app" "example" {
  id               = %q
  pingfederate_url = %q
  attribute_source = {
    entity_id = "urn:example:pingfederate"
    issuer    = %q
    jwks      = %s
  }
}
`, pingFederateAppId, pingFederateUrl, issuer, jwks)
}

// An attribute-source app needs no connector. P0 sets its audience, and a new issuer or
// pasted keys change it in place.
func TestPingFederateAwsIamFederationAttributeSource(t *testing.T) {
	const address = "p0_pingfederate_aws_iam_federation_app.example"
	const pasted = `{"keys":[{"kty":"RSA","e":"AQAB","n":"x"}]}`
	f := newPingFederateFederationFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if item := f.item(pingFederateFederation, "app", pingFederateAppId); item != nil {
				return fmt.Errorf("app is still in P0: %v", item)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: pingFederateAttributeSourceConfig(f, "urn:example:pingfederate:p0", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "state", "installed"),
					resource.TestCheckResourceAttr(address, "attribute_source.audience", pingFederateAudience),
					resource.TestCheckResourceAttr(address, "attribute_source.roles_url", f.server.URL+"/integrations/pingfederate-aws-iam-federation/"+fakeOrg),
					resource.TestCheckNoResourceAttr(address, "attribute_source.jwks"),
				),
			},
			{
				Config: pingFederateAttributeSourceConfig(f, "urn:example:pingfederate:p0-2", fmt.Sprintf("%q", pasted)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "attribute_source.issuer", "urn:example:pingfederate:p0-2"),
					resource.TestCheckResourceAttr(address, "attribute_source.jwks", pasted),
					resource.TestCheckResourceAttr(address, "attribute_source.audience", pingFederateAudience),
					func(*terraform.State) error {
						source, _ := f.item(pingFederateFederation, "app", pingFederateAppId)["roleSource"].(map[string]any)
						jwks, _ := source["jwks"].(map[string]any)
						if source["issuer"] != "urn:example:pingfederate:p0-2" || jwks["type"] != "keys" || jwks["keys"] != pasted {
							return fmt.Errorf("P0 item does not hold the configured role source: %v", source)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     pingFederateAppId,
				ImportStateVerify: true,
			},
		},
	})
}
