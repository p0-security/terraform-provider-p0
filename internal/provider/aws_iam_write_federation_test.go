package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const iamWriteAccountId = "123456789012"

func iamWriteConfig(f *fakeP0, login string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_aws_iam_write" "example" {
  id    = %q
  login = %s
}
`, iamWriteAccountId, login)
}

func TestAwsIamWriteFederationLogin(t *testing.T) {
	f := newFakeP0(t)
	f.integrations["aws"] = true
	f.items[fakeItemKey{"aws", "iam-write", iamWriteAccountId}] = map[string]any{"state": "stage"}
	login := `{
    type       = "aws-iam-federation"
    federation = "okta-prod"
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: iamWriteConfig(f, login),
				Check: func(*terraform.State) error {
					login, _ := f.item("aws", "iam-write", iamWriteAccountId)["login"].(map[string]any)
					if login["type"] != "aws-iam-federation" || login["federation"] != "okta-prod" {
						return fmt.Errorf("login = %v", login)
					}
					return nil
				},
			},
			{
				Config: iamWriteConfig(f, login),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAwsIamWriteFederationLoginValidation(t *testing.T) {
	f := newFakeP0(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      iamWriteConfig(f, `{ type = "aws-iam-federation" }`),
				ExpectError: regexp.MustCompile(`federation`),
			},
			{
				Config: iamWriteConfig(f, `{
    type       = "aws-iam-federation"
    federation = "Okta_Prod"
  }`),
				ExpectError: regexp.MustCompile(`lowercase letters`),
			},
			{
				Config: iamWriteConfig(f, `{
    type       = "aws-iam-federation"
    federation = "okta-prod"
    parent     = "210987654321"
  }`),
				ExpectError: regexp.MustCompile(`cannot be specified when`),
			},
		},
	})
}
