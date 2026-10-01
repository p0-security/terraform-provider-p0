package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const lambdaArn = "arn:aws:lambda:us-west-2:123456789012:function:connector"

// Contains a "/", so it only addresses the right item if the provider escapes it.
const exampleId = "primary/one"

func providerConfig(f *fakeP0) string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.server.URL)
}

// A function-caller staged for its role metadata and then installed, and a second
// item, which needs no staging, whose nested config points at the function.
func integrationItemConfig(f *fakeP0, serviceConfig string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_integration_item_staged" "caller" {
  integration = "aws"
  component   = "function-caller"
  id          = %q
}

resource "p0_integration_item" "caller" {
  integration = p0_integration_item_staged.caller.integration
  component   = p0_integration_item_staged.caller.component
  id          = p0_integration_item_staged.caller.id
}

resource "p0_integration_item" "resource" {
  integration = "example"
  component   = "iam-write"
  id          = %q
  config      = %s
}
`, lambdaArn, exampleId, serviceConfig)
}

func newIntegrationItemFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.metadata = func(key fakeItemKey, _ map[string]any) map[string]any {
		if key.integration != "aws" || key.component != "function-caller" {
			return nil
		}
		return map[string]any{
			"roleName":    "P0RoleIamFunctionCaller",
			"trustPolicy": `{"Version":"2012-10-17"}`,
		}
	}
	// Like P0's installers, add fields the config never sets: a computed top-level
	// field, and a default inside a nested select.
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "example" {
			return
		}
		item["accountId"] = "123456789012"
		if service, ok := item["service"].(map[string]any); ok {
			if _, ok := service["region"]; !ok {
				service["region"] = "us-west-2"
			}
		}
	}
	return f
}

func exampleService(f *fakeP0) map[string]any {
	service, _ := f.item("example", "iam-write", exampleId)["service"].(map[string]any)
	return service
}

func TestIntegrationItem(t *testing.T) {
	f := newIntegrationItemFake(t)
	lambdaConfig := `jsonencode({ service = { type = "aws", lambda = "aws:${p0_integration_item.caller.id}" } })`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			for _, key := range [][3]string{{"aws", "function-caller", lambdaArn}, {"example", "iam-write", exampleId}} {
				if item := f.item(key[0], key[1], key[2]); item != nil {
					return fmt.Errorf("%s was not removed from P0: %v", key, item)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			// Fields P0 adds must neither fail the apply nor leave a diff; the framework
			// re-plans after every step and fails on a non-empty plan.
			{
				Config: integrationItemConfig(f, lambdaConfig),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_integration_item_staged.caller", "metadata.%", "2"),
					resource.TestCheckResourceAttr("p0_integration_item_staged.caller", "metadata.roleName", "P0RoleIamFunctionCaller"),
					resource.TestCheckResourceAttr("p0_integration_item.caller", "state", "installed"),
					resource.TestCheckResourceAttr("p0_integration_item.caller", "label", lambdaArn),
					resource.TestCheckResourceAttr("p0_integration_item.resource", "state", "installed"),
					resource.TestCheckResourceAttr(
						"p0_integration_item.resource", "config",
						fmt.Sprintf(`{"service":{"lambda":"aws:%s","type":"aws"}}`, lambdaArn),
					),
					resource.TestCheckResourceAttr(
						"p0_integration_item.resource", "item",
						fmt.Sprintf(
							`{"accountId":"123456789012","label":%q,"service":{"lambda":"aws:%s","region":"us-west-2","type":"aws"},"state":"installed"}`,
							exampleId, lambdaArn,
						),
					),
				),
			},
			// A changed config updates the item in place.
			{
				Config: integrationItemConfig(f, `jsonencode({ service = { type = "gcp", cloudRun = "gcp:connector" } })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_integration_item.resource", plancheck.ResourceActionUpdate),
					},
				},
				Check: func(*terraform.State) error {
					service := exampleService(f)
					if service["type"] != "gcp" || service["cloudRun"] != "gcp:connector" {
						return fmt.Errorf("P0 item was not updated: %v", service)
					}
					return nil
				},
			},
			// A configured value changed outside Terraform shows as drift, and applying
			// restores it.
			{
				PreConfig: func() {
					f.update("example", "iam-write", exampleId, func(item map[string]any) {
						item["service"] = map[string]any{"type": "gcp", "cloudRun": "gcp:somewhere-else"}
					})
				},
				Config: integrationItemConfig(f, `jsonencode({ service = { type = "gcp", cloudRun = "gcp:connector" } })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_integration_item.resource", plancheck.ResourceActionUpdate),
					},
				},
				Check: func(*terraform.State) error {
					if got := exampleService(f)["cloudRun"]; got != "gcp:connector" {
						return fmt.Errorf("drift was not corrected, cloudRun = %v", got)
					}
					return nil
				},
			},
			// An item P0 no longer reports as installed (e.g. re-staged by an update to its
			// staged resource) is installed again.
			{
				PreConfig: func() {
					f.update("aws", "function-caller", lambdaArn, func(item map[string]any) {
						item["state"] = "stage"
					})
				},
				Config: integrationItemConfig(f, `jsonencode({ service = { type = "gcp", cloudRun = "gcp:connector" } })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_integration_item.caller", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_integration_item.caller", "state", "installed"),
			},
			// A config written as a differently formatted JSON string is adopted once, then
			// stays stable across refreshes.
			{
				Config: integrationItemConfig(f, `<<-EOT
    {
      "service": { "cloudRun": "gcp:connector", "type": "gcp" }
    }
  EOT`),
			},
			{
				ResourceName:            "p0_integration_item.resource",
				ImportState:             true,
				ImportStateId:           "example/iam-write/" + exampleId,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
			{
				ResourceName:      "p0_integration_item_staged.caller",
				ImportState:       true,
				ImportStateId:     "aws/function-caller/" + lambdaArn,
				ImportStateVerify: true,
			},
		},
	})
}

// Destroying only the final resource removes the item from P0. The staged resource
// then finds it gone, and converges by staging it again on the next apply.
func TestIntegrationItemRemovingFinalConverges(t *testing.T) {
	f := newIntegrationItemFake(t)
	staged := providerConfig(f) + fmt.Sprintf(`
resource "p0_integration_item_staged" "caller" {
  integration = "aws"
  component   = "function-caller"
  id          = %q
}
`, lambdaArn)
	withFinal := staged + `
resource "p0_integration_item" "caller" {
  integration = p0_integration_item_staged.caller.integration
  component   = p0_integration_item_staged.caller.component
  id          = p0_integration_item_staged.caller.id
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: withFinal},
			{
				Config: staged,
				// The staged resource's refresh finds the item gone and plans to stage it again.
				ExpectNonEmptyPlan: true,
				Check: func(*terraform.State) error {
					if item := f.item("aws", "function-caller", lambdaArn); item != nil {
						return fmt.Errorf("item was not removed: %v", item)
					}
					return nil
				},
			},
			{
				Config: staged,
				Check: func(*terraform.State) error {
					if got := f.item("aws", "function-caller", lambdaArn)["state"]; got != "stage" {
						return fmt.Errorf("item state = %v, want stage", got)
					}
					return nil
				},
			},
		},
	})
}

func TestIntegrationItemRejectsNonObjectConfig(t *testing.T) {
	f := newFakeP0(t)
	for name, config := range map[string]string{
		"array":   `jsonencode(["service"])`,
		"invalid": `"{ not json"`,
	} {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_integration_item" "bad" {
  integration = "example"
  component   = "iam-write"
  id          = "primary"
  config      = %s
}
`, config),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(`'config' must be a JSON object`),
					},
				},
			})
		})
	}
}
