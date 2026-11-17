package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	aiGatewayLogProviderAddress     = "p0_ai_gateway_log_provider.vl"
	aiGatewayLogProviderId          = "vl"
	aiGatewayLogProviderComponent   = "log-provider"
	aiGatewayLogProviderLinkAddress = "p0_ai_gateway_log_provider_link.primary"
	aiGatewayAddress                = "p0_ai_gateway.primary"
	aiGatewayId                     = "primary"
	aiGatewayComponent              = "gateway"
)

// Models how P0 rebuilds a log provider from its selected option's schema on each
// write: a victorialogs provider keeps no projectId. The fake only deep-merges.
func newAiGatewayLogProviderFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.component != aiGatewayLogProviderComponent {
			return
		}
		provider, _ := item["provider"].(map[string]any)
		if provider["type"] == "victorialogs" {
			delete(provider, "projectId")
		}
	}
	return f
}

// Stores a gateway in the given install state. A "stage" gateway is what
// p0_ai_gateway_staged leaves, so p0_ai_gateway can finish its install. An
// "installed" gateway can be linked without a p0_ai_gateway resource.
func seedAiGateway(f *fakeP0, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.integrations[aiGatewayIntegrationKey] = true
	f.items[fakeItemKey{aiGatewayIntegrationKey, aiGatewayComponent, aiGatewayId}] = map[string]any{
		"label":               aiGatewayId,
		"state":               state,
		"domainHosting":       map[string]any{"type": "selfHosted", "url": "https://gateway.example.com"},
		"serviceAccountEmail": "p0@example.iam.gserviceaccount.com",
	}
}

func aiGatewayLogProviderConfig(f *fakeP0, body string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_ai_gateway_log_provider" "vl" {
  id = %q
%s
}
`, aiGatewayLogProviderId, body)
}

// A gateway, and, unless logProvider is empty, two log providers and a link from the
// gateway to the one that logProvider names.
func aiGatewayLinkConfig(f *fakeP0, loadBalancerIp string, logProvider string) string {
	config := providerConfig(f) + fmt.Sprintf(`
resource "p0_ai_gateway" "primary" {
  id                  = %q
  domain_hosting      = { load_balancer_ip = %q }
  braekhus_public_jwk = jsonencode({ kty = "EC", crv = "P-384", x = "a", y = "b" })
}
`, aiGatewayId, loadBalancerIp)
	if logProvider == "" {
		return config
	}
	return config + fmt.Sprintf(`
resource "p0_ai_gateway_log_provider" "vl" {
  id   = "vl"
  type = "victorialogs"
}

resource "p0_ai_gateway_log_provider" "gcp" {
  id         = "gcp"
  type       = "gcp"
  project_id = "my-logging-project"
}

resource "p0_ai_gateway_log_provider_link" "primary" {
  gateway      = p0_ai_gateway.primary.id
  log_provider = p0_ai_gateway_log_provider.%s.id
}
`, logProvider)
}

func checkAiGatewayLogProviderItem(f *fakeP0, wantType string, wantProjectId any) func(*terraform.State) error {
	return func(*terraform.State) error {
		item := f.item(aiGatewayIntegrationKey, aiGatewayLogProviderComponent, aiGatewayLogProviderId)
		provider, _ := item["provider"].(map[string]any)
		if provider["type"] != wantType || provider["projectId"] != wantProjectId || item["state"] != "installed" {
			return fmt.Errorf("P0's log provider = %v; want an installed %q provider with projectId %v", item, wantType, wantProjectId)
		}
		return nil
	}
}

func checkAiGatewayLogProviderGone(f *fakeP0, ids ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		for _, id := range ids {
			if item := f.item(aiGatewayIntegrationKey, aiGatewayLogProviderComponent, id); item != nil {
				return fmt.Errorf("log provider %q was not removed from P0: %v", id, item)
			}
		}
		return nil
	}
}

func checkAiGatewayLinkedTo(f *fakeP0, want string) func(*terraform.State) error {
	return func(*terraform.State) error {
		item := f.item(aiGatewayIntegrationKey, aiGatewayComponent, aiGatewayId)
		if got, ok := item["log-provider"]; !ok || got != want {
			return fmt.Errorf("P0's gateway log-provider = %v; want %q (%v)", got, want, item)
		}
		return nil
	}
}

func TestAiGatewayLogProvider(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayLogProviderGone(f, aiGatewayLogProviderId),
		Steps: []resource.TestStep{
			{
				Config: aiGatewayLogProviderConfig(f, `  type = "victorialogs"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayLogProviderAddress, "type", "victorialogs"),
					resource.TestCheckNoResourceAttr(aiGatewayLogProviderAddress, "project_id"),
					checkAiGatewayLogProviderItem(f, "victorialogs", nil),
				),
			},
			// An installed log provider can switch type in place.
			{
				Config: aiGatewayLogProviderConfig(f, `  type       = "gcp"
  project_id = "my-logging-project"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayLogProviderAddress, "type", "gcp"),
					resource.TestCheckResourceAttr(aiGatewayLogProviderAddress, "project_id", "my-logging-project"),
					checkAiGatewayLogProviderItem(f, "gcp", "my-logging-project"),
				),
			},
			{
				ResourceName:      aiGatewayLogProviderAddress,
				ImportState:       true,
				ImportStateId:     aiGatewayLogProviderId,
				ImportStateVerify: true,
			},
			// And back, which drops the project.
			{
				Config: aiGatewayLogProviderConfig(f, `  type = "victorialogs"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(aiGatewayLogProviderAddress, "project_id"),
					checkAiGatewayLogProviderItem(f, "victorialogs", nil),
				),
			},
		},
	})
}

func TestAiGatewayLogProviderProjectIdMatchesType(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: aiGatewayLogProviderConfig(f, `  type       = "victorialogs"
  project_id = "my-logging-project"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("`project_id` is not valid when `type` is \"victorialogs\""),
			},
			{
				Config:      aiGatewayLogProviderConfig(f, `  type = "gcp"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("`project_id` is required when `type` is \"gcp\""),
			},
			{
				Config: aiGatewayLogProviderConfig(f, `  type       = "gcp"
  project_id = ""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

// Covers create, change, import and destroy of a link, re-applying the gateway with
// the link in place, and destroying the link and its log providers in one apply.
func TestAiGatewayLogProviderLink(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)
	seedAiGateway(f, "stage")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: aiGatewayLinkConfig(f, "10.0.0.1", "vl"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayLogProviderLinkAddress, "id", aiGatewayId),
					resource.TestCheckResourceAttr(aiGatewayLogProviderLinkAddress, "log_provider", "vl"),
					checkAiGatewayLinkedTo(f, "vl"),
				),
			},
			{
				Config: aiGatewayLinkConfig(f, "10.0.0.1", "gcp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderLinkAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiGatewayLogProviderLinkAddress, "log_provider", "gcp"),
					checkAiGatewayLinkedTo(f, "gcp"),
				),
			},
			{
				ResourceName:      aiGatewayLogProviderLinkAddress,
				ImportState:       true,
				ImportStateId:     aiGatewayId,
				ImportStateVerify: true,
			},
			// p0_ai_gateway never sends log-provider, so re-applying it keeps the link.
			{
				Config: aiGatewayLinkConfig(f, "10.0.0.2", "gcp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayAddress, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(aiGatewayLogProviderLinkAddress, plancheck.ResourceActionNoop),
					},
				},
				Check: checkAiGatewayLinkedTo(f, "gcp"),
			},
			// Terraform destroys the link before the log provider it refers to, so the
			// gateway holds no dangling id.
			{
				Config: aiGatewayLinkConfig(f, "10.0.0.2", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAiGatewayLinkedTo(f, ""),
					checkAiGatewayLogProviderGone(f, "vl", "gcp"),
				),
			},
		},
	})
}

// A gateway unlinked outside Terraform makes the next plan create the link again.
func TestAiGatewayLogProviderLinkRemovedOutsideTerraform(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)
	seedAiGateway(f, "stage")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			checkAiGatewayLinkedTo(f, ""),
			checkAiGatewayLogProviderGone(f, "vl", "gcp"),
		),
		Steps: []resource.TestStep{
			{
				Config: aiGatewayLinkConfig(f, "10.0.0.1", "vl"),
				Check:  checkAiGatewayLinkedTo(f, "vl"),
			},
			{
				PreConfig: func() {
					f.update(aiGatewayIntegrationKey, aiGatewayComponent, aiGatewayId, func(item map[string]any) {
						item["log-provider"] = ""
					})
				},
				Config: aiGatewayLinkConfig(f, "10.0.0.1", "vl"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderLinkAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: checkAiGatewayLinkedTo(f, "vl"),
			},
		},
	})
}

// Two log providers and, if linked, a link from the installed gateway to the one
// that logProvider names. It has no p0_ai_gateway, so the gateway must be seeded.
func aiGatewayLinkOnlyConfig(f *fakeP0, logProvider string, linked bool) string {
	config := providerConfig(f) + `
resource "p0_ai_gateway_log_provider" "vl" {
  id   = "vl"
  type = "victorialogs"
}

resource "p0_ai_gateway_log_provider" "gcp" {
  id         = "gcp"
  type       = "gcp"
  project_id = "my-logging-project"
}
`
	if !linked {
		return config
	}
	return config + fmt.Sprintf(`
resource "p0_ai_gateway_log_provider_link" "primary" {
  gateway      = %q
  log_provider = p0_ai_gateway_log_provider.%s.id
}
`, aiGatewayId, logProvider)
}

func checkNoResource(address string) func(*terraform.State) error {
	return func(s *terraform.State) error {
		if _, ok := s.RootModule().Resources[address]; ok {
			return fmt.Errorf("%s is still in state", address)
		}
		return nil
	}
}

// P0 checks that it can reach the gateway on each gateway configure. A refused link,
// change or unlink fails with a message that names reachability, and leaves P0's
// gateway as it was. The next apply tries again.
func TestAiGatewayLogProviderLinkRefused(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)
	seedAiGateway(f, "installed")
	var unreachable atomic.Bool
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if _, links := step.body["log-provider"]; links && unreachable.Load() && step.key.component == aiGatewayComponent {
			return http.StatusBadRequest, "Gateway is not reachable"
		}
		return 0, ""
	}
	refused := func(summary string) *regexp.Regexp {
		return regexp.MustCompile(summary + `(?s).*management\s+tunnel.*Gateway\s+is\s+not\s+reachable`)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayLogProviderGone(f, "vl", "gcp"),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { unreachable.Store(true) },
				Config:      aiGatewayLinkOnlyConfig(f, "vl", true),
				ExpectError: refused(`Could not link the AI gateway's log provider`),
			},
			{
				PreConfig: func() { unreachable.Store(false) },
				Config:    aiGatewayLinkOnlyConfig(f, "vl", true),
				Check:     checkAiGatewayLinkedTo(f, "vl"),
			},
			{
				PreConfig:   func() { unreachable.Store(true) },
				Config:      aiGatewayLinkOnlyConfig(f, "gcp", true),
				ExpectError: refused(`Could not link the AI gateway's log provider`),
			},
			{
				PreConfig: func() {
					unreachable.Store(false)
					if err := checkAiGatewayLinkedTo(f, "vl")(nil); err != nil {
						t.Errorf("after the refused change: %v", err)
					}
				},
				Config: aiGatewayLinkOnlyConfig(f, "gcp", true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderLinkAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: checkAiGatewayLinkedTo(f, "gcp"),
			},
			{
				PreConfig:   func() { unreachable.Store(true) },
				Config:      aiGatewayLinkOnlyConfig(f, "gcp", false),
				ExpectError: refused(`Could not unlink the AI gateway's log provider`),
			},
			{
				PreConfig: func() { unreachable.Store(false) },
				Config:    aiGatewayLinkOnlyConfig(f, "gcp", false),
				Check:     checkAiGatewayLinkedTo(f, ""),
			},
		},
	})
}

// A link to a gateway that P0 does not have says so.
func TestAiGatewayLogProviderLinkGatewayNotFound(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayLogProviderGone(f, "vl", "gcp"),
		Steps: []resource.TestStep{
			{
				Config:      aiGatewayLinkOnlyConfig(f, "vl", true),
				ExpectError: regexp.MustCompile(`P0\s+has\s+no\s+gateway\s+"primary"`),
			},
		},
	})
}

// A gateway removed from P0 drops its link at refresh, so the next plan creates it.
// A gateway removed between refresh and destroy needs no unlink, so the destroy
// succeeds.
func TestAiGatewayLogProviderLinkGatewayGone(t *testing.T) {
	f := newAiGatewayLogProviderFake(t)
	seedAiGateway(f, "installed")
	var gatewayGone atomic.Bool
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if gatewayGone.Load() && step.key.component == aiGatewayComponent {
			return http.StatusNotFound, "Not found"
		}
		return 0, ""
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAiGatewayLogProviderGone(f, "vl", "gcp"),
		Steps: []resource.TestStep{
			{
				Config: aiGatewayLinkOnlyConfig(f, "vl", true),
				Check:  checkAiGatewayLinkedTo(f, "vl"),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.items, fakeItemKey{aiGatewayIntegrationKey, aiGatewayComponent, aiGatewayId})
				},
				Config:             aiGatewayLinkOnlyConfig(f, "vl", true),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: func() { seedAiGateway(f, "installed") },
				Config:    aiGatewayLinkOnlyConfig(f, "vl", true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aiGatewayLogProviderLinkAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: checkAiGatewayLinkedTo(f, "vl"),
			},
			// The refresh still finds the gateway, but the unlink gets a 404.
			{
				PreConfig: func() { gatewayGone.Store(true) },
				Config:    aiGatewayLinkOnlyConfig(f, "vl", false),
				Check:     checkNoResource(aiGatewayLogProviderLinkAddress),
			},
		},
	})
}
