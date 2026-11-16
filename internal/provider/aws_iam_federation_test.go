package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	federationStaged = "p0_aws_iam_federation_staged.example"
	federation       = "p0_aws_iam_federation.example"
)

func newFederationFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.metadata = func(key fakeItemKey, _ map[string]any) map[string]any {
		return map[string]any{
			"roleName":            "P0Federation-" + key.id,
			"trustPolicy":         `{"trust":true}`,
			"inlinePolicy":        `{"inline":true}`,
			"samlProviderName":    "P0Federation-" + key.id,
			"poolPath":            "/P0Federation-" + key.id + "/",
			"poolRoleTrustPolicy": `{"pool":true}`,
			"poolRoleSshPolicy":   `{"ssh":true}`,
		}
	}
	return f
}

func federationConfig(f *fakeP0, extra string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_aws_iam_federation_staged" "example" {
  id                = "okta-prod"
  account_id        = "123456789012"
  identity_provider = "okta"
  app               = "0oa1b2c3d4e5f6g7h8i9"
  %s
}

resource "p0_aws_iam_federation" "example" {
  id                = p0_aws_iam_federation_staged.example.id
  account_id        = p0_aws_iam_federation_staged.example.account_id
  partition         = p0_aws_iam_federation_staged.example.partition
  pool_size         = p0_aws_iam_federation_staged.example.pool_size
  identity_provider = p0_aws_iam_federation_staged.example.identity_provider
  app               = p0_aws_iam_federation_staged.example.app
}
`, extra)
}

func checkFederationItem(f *fakeP0, want map[string]any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		item := f.item("aws-iam-federation", "federation", "okta-prod")
		for field, value := range want {
			if fmt.Sprint(item[field]) != fmt.Sprint(value) {
				return fmt.Errorf("item.%s = %v, want %v", field, item[field], value)
			}
		}
		return nil
	}
}

func TestAwsIamFederation(t *testing.T) {
	f := newFederationFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if item := f.item("aws-iam-federation", "federation", "okta-prod"); item != nil {
				return fmt.Errorf("the federation is still in P0: %v", item)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: federationConfig(f, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkFederationItem(f, map[string]any{
						"state":        "installed",
						"accountId":    "123456789012",
						"awsPartition": map[string]any{"type": "aws"},
						"poolSize":     "10",
						"provider":     map[string]any{"type": "okta", "app": "0oa1b2c3d4e5f6g7h8i9"},
					}),
					resource.TestCheckResourceAttr(federationStaged, "role.name", "P0Federation-okta-prod"),
					resource.TestCheckResourceAttr(federationStaged, "role.trust_policy", `{"trust":true}`),
					resource.TestCheckResourceAttr(federationStaged, "saml_provider_name", "P0Federation-okta-prod"),
					resource.TestCheckResourceAttr(federationStaged, "pool.path", "/P0Federation-okta-prod/"),
					resource.TestCheckResourceAttr(federationStaged, "pool.ssh_policy", `{"ssh":true}`),
					resource.TestCheckResourceAttr(federationStaged, "pool_size", "10"),
					resource.TestCheckResourceAttr(federation, "state", "installed"),
				),
			},
			{
				Config: federationConfig(f, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            federationStaged,
				ImportState:             true,
				ImportStateId:           "okta-prod",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				Config: federationConfig(f, "pool_size = 3"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(federationStaged, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(federation, plancheck.ResourceActionReplace),
					},
				},
				Check: checkFederationItem(f, map[string]any{"state": "installed", "poolSize": "3"}),
			},
		},
	})
}
