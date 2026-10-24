package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	aiGatewayServerAddress   = "p0_ai_gateway_server.example"
	aiGatewayServerId        = "workspace-tools"
	aiGatewayIntegrationKey  = "agentic"
	aiGatewayServerComponent = "server"
)

// A predefined server, with `instance = <instance>` in its definition unless instance
// is empty.
func aiGatewayServerConfig(f *fakeP0, instance string) string {
	instanceLine := ""
	if instance != "" {
		instanceLine = fmt.Sprintf("\n    instance = %q", instance)
	}
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_ai_gateway_server" "example" {
  id         = %q
  gateway    = "primary"
  credential = { type = "generic" }
  definition = {
    type = "p0"
    id   = "example"%s
  }
}
`, aiGatewayServerId, instanceLine)
}

// Refuses a verify or configure that changes the definition's instance, as P0 does
// for a "step: new" field. A server staged without its instance fails here.
func newAiGatewayServerFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.nextState == "stage" {
			return 0, ""
		}
		previous, _ := step.previous["definition"].(map[string]any)
		next, _ := step.body["definition"].(map[string]any)
		if previous["instance"] != next["instance"] {
			return http.StatusBadRequest, "'definition/p0/instance' can only be altered on initial installation."
		}
		return 0, ""
	}
	return f
}

func checkAiGatewayServerInstance(f *fakeP0, want string) func(*terraform.State) error {
	return func(*terraform.State) error {
		item := f.item(aiGatewayIntegrationKey, aiGatewayServerComponent, aiGatewayServerId)
		definition, _ := item["definition"].(map[string]any)
		instance, ok := definition["instance"]
		if want == "" && ok {
			return fmt.Errorf("P0 item has an instance: %v", item)
		}
		if want != "" && instance != want {
			return fmt.Errorf("P0 item's definition.instance = %v; want %q (%v)", instance, want, item)
		}
		return nil
	}
}

func checkAiGatewayServerDestroyed(f *fakeP0) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.item(aiGatewayIntegrationKey, aiGatewayServerComponent, aiGatewayServerId); item != nil {
			return fmt.Errorf("the MCP server was not removed from P0: %v", item)
		}
		return nil
	}
}

func TestAiGatewayServerInstance(t *testing.T) {
	f := newAiGatewayServerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayServerDestroyed(f),
		Steps: []resource.TestStep{
			// The framework refreshes and re-plans after the step and fails on a non-empty
			// plan, so this also checks that the instance P0 stores reads back.
			{
				Config: aiGatewayServerConfig(f, "acme"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayServerAddress, "definition.instance", "acme"),
					checkAiGatewayServerInstance(f, "acme"),
				),
			},
			// P0 fixes the instance when the server is added, so a new one replaces it.
			{
				Config: aiGatewayServerConfig(f, "globex"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayServerAddress, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayServerAddress, "definition.instance", "globex"),
					checkAiGatewayServerInstance(f, "globex"),
				),
			},
		},
	})
}

// A server without an instance stages only its gateway, as before, and P0 stores no
// instance.
func TestAiGatewayServerWithoutInstance(t *testing.T) {
	f := newAiGatewayServerFake(t)
	refuseInstance := f.refuse
	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		if _, ok := step.body["definition"]; ok && step.nextState == "stage" {
			return http.StatusBadRequest, "staging sent a definition"
		}
		return refuseInstance(step, items)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayServerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: aiGatewayServerConfig(f, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(aiGatewayServerAddress, "definition.instance"),
					checkAiGatewayServerInstance(f, ""),
				),
			},
		},
	})
}
