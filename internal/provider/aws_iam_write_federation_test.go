package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	iamWrite          = "p0_aws_iam_write.example"
	iamWriteAccountId = "123456789012"
)

var legacyOktaProvider = map[string]any{
	"type":             "okta",
	"appId":            "0oa111",
	"identityProvider": "okta-saml",
	"method":           map[string]any{"type": "saml", "accountCount": map[string]any{"type": "single"}},
}

// Stages the account, and adds what P0's AWS installer does for a login that refers to
// a federation app: it stores the app's `provider` block in the login.
func newIamWriteFederationFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.integrations["aws"] = true
	f.items[fakeItemKey{"aws", "iam-write", iamWriteAccountId}] = map[string]any{"state": "stage"}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		login, _ := item["login"].(map[string]any)
		if key.component == "iam-write" && login["federation"] != nil {
			login["provider"] = legacyOktaProvider
		}
	}
	return f
}

func iamWriteConfig(f *fakeP0, partition, login string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_aws_iam_write" "example" {
  id        = %q
  partition = %q
  login     = %s
}
`, iamWriteAccountId, partition, login)
}

const legacyLogin = `{
    type = "federated"
    provider = {
      app_id            = "0oa111"
      identity_provider = "okta-saml"
      method = {
        account_count = {}
      }
    }
  }`

const itemLogin = `{
    type          = "federated"
    federation    = "okta-prod"
    saml_provider = "okta-saml"
  }`

func checkIamWriteLogin(f *fakeP0, field string, want any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		item := f.item("aws", "iam-write", iamWriteAccountId)
		login, _ := item["login"].(map[string]any)
		if fmt.Sprint(login[field]) != fmt.Sprint(want) {
			return fmt.Errorf("login.%s = %v, want %v", field, login[field], want)
		}
		return nil
	}
}

func TestAwsIamWriteFederationLogin(t *testing.T) {
	f := newIamWriteFederationFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: iamWriteConfig(f, "aws", itemLogin),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkIamWriteLogin(f, "federation", "okta-prod"),
					checkIamWriteLogin(f, "samlProvider", "okta-saml"),
					resource.TestCheckNoResourceAttr(iamWrite, "login.provider"),
				),
			},
			{
				Config: iamWriteConfig(f, "aws", itemLogin),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAwsIamWriteLegacyLogin(t *testing.T) {
	f := newIamWriteFederationFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: iamWriteConfig(f, "aws", legacyLogin),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(iamWrite, "login.provider.app_id", "0oa111"),
					checkIamWriteLogin(f, "federation", nil),
					resource.TestCheckNoResourceAttr(iamWrite, "login.federation"),
				),
			},
			// P0 moves the login to a federation app. A config that still holds the
			// provider shows no change, and an update keeps the federation app in P0.
			{
				PreConfig: func() {
					f.update("aws", "iam-write", iamWriteAccountId, func(item map[string]any) {
						login := item["login"].(map[string]any)
						login["federation"] = "okta-prod"
						login["samlProvider"] = "okta-saml"
					})
				},
				Config: iamWriteConfig(f, "aws", legacyLogin),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: iamWriteConfig(f, "aws-us-gov", legacyLogin),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkIamWriteLogin(f, "federation", "okta-prod"),
					resource.TestCheckNoResourceAttr(iamWrite, "login.federation"),
				),
			},
			{
				Config: iamWriteConfig(f, "aws-us-gov", itemLogin),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkIamWriteLogin(f, "federation", "okta-prod"),
					resource.TestCheckResourceAttr(iamWrite, "login.federation", "okta-prod"),
					resource.TestCheckNoResourceAttr(iamWrite, "login.provider"),
				),
			},
		},
	})
}
