package provider

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const awsFederation = "p0_aws_federation.example"

func awsFederationConfig(f *fakeP0, appId, roleLabelFormat string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_aws_federation" "example" {
  id = "okta-prod"
  okta = {
    app_id            = %q
    role_label_format = %q
  }
}
`, appId, roleLabelFormat)
}

func checkAwsFederationItem(f *fakeP0, want map[string]any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		item := f.item("aws-federation", "federation", "okta-prod")
		if item == nil {
			return fmt.Errorf("the federation app is not in P0")
		}
		if !reflect.DeepEqual(item["provider"], want) {
			return fmt.Errorf("provider = %v, want %v", item["provider"], want)
		}
		if item["state"] != "installed" {
			return fmt.Errorf("state = %v, want installed", item["state"])
		}
		return nil
	}
}

func TestAwsFederation(t *testing.T) {
	f := newFakeP0(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if item := f.item("aws-federation", "federation", "okta-prod"); item != nil {
				return fmt.Errorf("the federation app was not removed from P0: %v", item)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: awsFederationConfig(f, "0oa111", "role-name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAwsFederationItem(f, map[string]any{
						"type":            "okta",
						"appId":           "0oa111",
						"roleLabelFormat": map[string]any{"type": "role-name"},
					}),
					resource.TestCheckResourceAttr(awsFederation, "okta.app_id", "0oa111"),
					resource.TestCheckResourceAttr(awsFederation, "state", "installed"),
				),
			},
			{
				ResourceName:      awsFederation,
				ImportState:       true,
				ImportStateId:     "okta-prod",
				ImportStateVerify: true,
			},
			// P0 refuses to change the provider of an installed app.
			{
				Config: awsFederationConfig(f, "0oa222", "alias-and-role-name"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(awsFederation, plancheck.ResourceActionReplace),
					},
				},
				Check: checkAwsFederationItem(f, map[string]any{
					"type":            "okta",
					"appId":           "0oa222",
					"roleLabelFormat": map[string]any{"type": "alias-and-role-name"},
				}),
			},
		},
	})
}
