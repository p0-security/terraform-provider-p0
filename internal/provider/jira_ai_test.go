package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	jiraAiConnector      = "prod"
	jiraAiConnectorName  = "p0-jira-prod"
	jiraAiProject        = "acme-prod"
	jiraAiServiceAccount = jiraAiConnectorName + "@" + jiraAiProject + ".iam.gserviceaccount.com"
	jiraAiServiceUri     = "https://" + jiraAiConnectorName + "-abc123-uc.a.run.app"
	jiraAiSite           = "acme"
	jiraAiCloudId        = "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"
	jiraAiPoolAccount    = "agent-1"
	jiraAiAccountId      = "712020:0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"
	jiraAiSecretName     = "p0_install_jira-ai_acme_agent-1_api-key"
)

// Where a Jira connector runs, as a customer's Terraform sets it, in a region.
func jiraAiHosting(region string) string {
	return fmt.Sprintf(`{
    type             = "gcp"
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, jiraAiProject, jiraAiConnectorName, region)
}

var jiraAiGcp = jiraAiHosting("us-central1")

func jiraAiStaged(hosting string) string {
	return fmt.Sprintf(`
resource "p0_jira_ai_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, jiraAiConnector, hosting)
}

// A connector staged and then installed. Each resource writes its hosting out: the
// staged one's hosting can't be assigned whole, since that sets its read-only
// attributes.
func jiraAiConnectorConfig(f *jiraAiFake, hosting string) string {
	return providerConfig(f.fakeP0) + jiraAiStaged(hosting) + fmt.Sprintf(`
resource "p0_jira_ai_connector" "example" {
  id      = p0_jira_ai_connector_staged.example.id
  hosting = %s
}
`, hosting)
}

func jiraAiSiteBlock(connector string) string {
	return fmt.Sprintf(`
resource "p0_jira_ai_site" "example" {
  id        = %q
  connector = %s
  site_url  = "https://acme.atlassian.net"

  vault = {
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }

  admin_client_id   = "admin-client"
  admin_secret_name = "jira-acme-admin"
}
`, jiraAiSite, connector)
}

const jiraAiPoolAccountBlock = `
resource "p0_jira_ai_pool_account" "example" {
  id    = "agent-1"
  site  = p0_jira_ai_site.example.id
  email = "agent-1@serviceaccount.atlassian.com"
}
`

// The whole chain, as a customer's Terraform installs it.
func jiraAiConfig(f *jiraAiFake) string {
	return jiraAiConnectorConfig(f, jiraAiGcp) + jiraAiSiteBlock("p0_jira_ai_connector.example.id") + jiraAiPoolAccountBlock
}

// Behaves as P0's Jira (AI) installers do (app#7540):
//   - staging a connector names the service account it must run as, and verifying it
//     looks up its Cloud Run URL. Verify and configure refuse a hosting that isn't the
//     staged one.
//   - staging a site needs an installed connector, and looks up the site's cloud ID
//   - staging an agent account names its secret, and verifying it needs an installed
//     site and looks the account's ID up through the connector
//
// The fields make it fail, for the test of each failure.
type jiraAiFake struct {
	*fakeP0
	// The service account P0 names for a connector, or nil for none.
	serviceAccount func(hosting map[string]any) any
	// P0's message for an agent account whose lookup fails, on verify.
	failedLookup string
	// How many writes to Jira (AI) items P0 got, refused or not.
	writes int
}

// The fields of a connector's hosting that P0 fixes when it creates the item.
var jiraAiAddressFields = []string{"type", "projectId", "connectorName", "connectorRegion"}

func newJiraAiFake(t *testing.T) *jiraAiFake {
	f := &jiraAiFake{fakeP0: newFakeP0(t)}
	f.serviceAccount = func(hosting map[string]any) any {
		return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", hosting["connectorName"], hosting["projectId"])
	}

	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "jira-ai" {
			return 0, ""
		}
		f.writes++
		installed := func(component string, id any) bool {
			key, _ := id.(string)
			item := items[fakeItemKey{"jira-ai", component, key}]
			return item != nil && item["state"] == "installed"
		}

		switch step.key.component {
		case "connector":
			if step.nextState == "stage" {
				return 0, ""
			}
			hosting, _ := step.body["hosting"].(map[string]any)
			staged, _ := step.previous["hosting"].(map[string]any)
			for _, field := range jiraAiAddressFields {
				if sent, ok := hosting[field]; ok && sent != staged[field] {
					return http.StatusUnprocessableEntity, fmt.Sprintf(
						"'hosting/gcp/%s' can only be altered on initial installation. Create a new installation to change this field.", field,
					)
				}
			}
		case "site":
			if step.nextState == "stage" && !installed("connector", step.body["connector"]) {
				return http.StatusBadRequest, fmt.Sprintf("The Jira connector %v, which the site %s uses, isn't installed. Finish installing it, then retry.",
					step.body["connector"], step.key.id)
			}
		case "pool-account":
			if step.nextState != "configure" {
				return 0, ""
			}
			if !installed("site", step.previous["site"]) {
				return http.StatusBadRequest, fmt.Sprintf("The Jira site %v, which the agent account %s uses, isn't installed. Finish installing it, then retry.",
					step.previous["site"], step.key.id)
			}
			if f.failedLookup != "" {
				return http.StatusUnprocessableEntity, f.failedLookup
			}
		}
		return 0, ""
	}

	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "jira-ai" {
			return
		}
		switch key.component {
		case "connector":
			hosting, ok := item["hosting"].(map[string]any)
			if !ok {
				return
			}
			if item["state"] == "stage" {
				delete(hosting, "connectorServiceUri")
				if account := f.serviceAccount(hosting); account != nil {
					hosting["connectorServiceAccount"] = account
				}
			} else {
				hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uc.a.run.app", hosting["connectorName"])
			}
		case "site":
			item["cloudId"] = jiraAiCloudId
		case "pool-account":
			item["secretName"] = fmt.Sprintf("p0_install_jira-ai_%s_%s_api-key", item["site"], key.id)
			if item["state"] != "stage" {
				item["accountId"] = jiraAiAccountId
			}
		}
	}
	return f
}

func (f *jiraAiFake) jiraItem(component, id string) map[string]any {
	return f.item("jira-ai", component, id)
}

func checkJiraAiDestroyed(f *jiraAiFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		for _, key := range []fakeItemKey{
			{"jira-ai", "connector", jiraAiConnector},
			{"jira-ai", "site", jiraAiSite},
			{"jira-ai", "pool-account", jiraAiPoolAccount},
		} {
			if item := f.jiraItem(key.component, key.id); item != nil {
				return fmt.Errorf("the %s %s was not removed from P0: %v", key.component, key.id, item)
			}
		}
		return nil
	}
}

// A copy of harborError in harbor_test.go.
//
// Terraform wraps a diagnostic's text, so the words of each part of an error may be on
// different lines. The parts match in order, with anything between them.
func jiraAiError(parts ...string) *regexp.Regexp {
	patterns := []string{}
	for _, part := range parts {
		patterns = append(patterns, strings.Join(strings.Fields(regexp.QuoteMeta(part)), `\s+`))
	}
	return regexp.MustCompile(`(?s)` + strings.Join(patterns, `.*`))
}

// The whole chain: the connector is staged and installed, then a site and an agent
// account on it. Each reads back what P0 sets, and imports as it was created.
func TestJiraAiFullChain(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: jiraAiConfig(f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira_ai_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_jira_ai_connector_staged.example", "hosting.connector_service_account", jiraAiServiceAccount),
					resource.TestCheckNoResourceAttr("p0_jira_ai_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_jira_ai_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira_ai_connector.example", "hosting.connector_service_account", jiraAiServiceAccount),
					resource.TestCheckResourceAttr("p0_jira_ai_connector.example", "hosting.connector_service_uri", jiraAiServiceUri),
					resource.TestCheckResourceAttr("p0_jira_ai_site.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira_ai_site.example", "cloud_id", jiraAiCloudId),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "account_id", jiraAiAccountId),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "secret_name", jiraAiSecretName),
					func(*terraform.State) error {
						hosting, _ := f.jiraItem("connector", jiraAiConnector)["hosting"].(map[string]any)
						if _, ok := hosting["accountId"]; ok || hosting["type"] != "gcp" || hosting["projectId"] != jiraAiProject {
							return fmt.Errorf("P0 connector does not hold the configured hosting: %v", hosting)
						}
						site := f.jiraItem("site", jiraAiSite)
						vault, _ := site["vault"].(map[string]any)
						if site["connector"] != jiraAiConnector || site["siteUrl"] != "https://acme.atlassian.net" || site["adminClientId"] != "admin-client" ||
							site["adminSecretName"] != "jira-acme-admin" ||
							vault["type"] != "gcp-sm" || vault["install"] != "acme-secrets" || len(vault) != 2 {
							return fmt.Errorf("P0 site does not hold the configured site: %v", site)
						}
						account := f.jiraItem("pool-account", jiraAiPoolAccount)
						if account["site"] != jiraAiSite || account["email"] != "agent-1@serviceaccount.atlassian.com" {
							return fmt.Errorf("P0 agent account does not hold the configured account: %v", account)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_jira_ai_connector_staged.example",
				ImportState:       true,
				ImportStateId:     jiraAiConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state", "hosting.connector_service_uri"},
			},
			{
				ResourceName:      "p0_jira_ai_connector.example",
				ImportState:       true,
				ImportStateId:     jiraAiConnector,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_jira_ai_site.example",
				ImportState:       true,
				ImportStateId:     jiraAiSite,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "p0_jira_ai_pool_account.example",
				ImportState:       true,
				ImportStateId:     jiraAiPoolAccount,
				ImportStateVerify: true,
			},
		},
	})
}

// Destroying only the final connector resource returns its item to "stage", so the
// staged resource still owns it, and a later apply installs it again.
func TestJiraAiConnectorRollback(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraAiConnectorConfig(f, jiraAiGcp)},
			{
				Config: providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp),
				Check: func(*terraform.State) error {
					item := f.jiraItem("connector", jiraAiConnector)
					hosting, _ := item["hosting"].(map[string]any)
					if item["state"] != "stage" || hosting["connectorServiceAccount"] != jiraAiServiceAccount {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
			{
				Config: jiraAiConnectorConfig(f, jiraAiGcp),
				Check:  resource.TestCheckResourceAttr("p0_jira_ai_connector.example", "state", "installed"),
			},
		},
	})
}

// Where a connector runs is fixed when it is added, so a new region replaces the
// staged resource, and the final resource with it, in one apply.
func TestJiraAiConnectorHostingReplacesBoth(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraAiConnectorConfig(f, jiraAiGcp)},
			{
				Config: jiraAiConnectorConfig(f, jiraAiHosting("europe-west1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira_ai_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_jira_ai_connector.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					item := f.jiraItem("connector", jiraAiConnector)
					hosting, _ := item["hosting"].(map[string]any)
					if item["state"] != "installed" || hosting["connectorRegion"] != "europe-west1" {
						return fmt.Errorf("P0 item is not the connector installed where it moved to: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// The final connector verifies the hosting it names, and P0 refuses one that isn't
// the staged one, so the connector stays where it was staged.
func TestJiraAiConnectorMustMatchStaged(t *testing.T) {
	f := newJiraAiFake(t)
	config := providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp) + fmt.Sprintf(`
resource "p0_jira_ai_connector" "example" {
  id      = p0_jira_ai_connector_staged.example.id
  hosting = %s
}
`, jiraAiHosting("europe-west1"))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: jiraAiError("Jira connector install check failed", "'hosting/gcp/connectorRegion' can only be altered on initial installation"),
			},
			{
				PreConfig: func() {
					item := f.jiraItem("connector", jiraAiConnector)
					if hosting, _ := item["hosting"].(map[string]any); item["state"] != "stage" || hosting["connectorRegion"] != "us-central1" {
						t.Errorf("P0 item = %v; want it staged where it was staged", item)
					}
				},
				Config: providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp),
			},
		},
	})
}

// The connector can't be deployed without the service account P0 names, so a connector
// that comes back without one fails the apply. The connector P0 staged isn't left
// behind, so once P0 returns the account the next apply stages it.
func TestJiraAiConnectorMissingServiceAccount(t *testing.T) {
	f := newJiraAiFake(t)
	working := f.serviceAccount
	f.serviceAccount = func(map[string]any) any { return nil }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp),
				ExpectError: jiraAiError("Bad API response", "P0 returned the Jira connector prod without its service account"),
			},
			{
				PreConfig: func() {
					if item := f.jiraItem("connector", jiraAiConnector); item != nil {
						t.Errorf("P0 item after the failed staging = %v; want none", item)
					}
					f.serviceAccount = working
				},
				Config: providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp),
				Check:  resource.TestCheckResourceAttr("p0_jira_ai_connector_staged.example", "hosting.connector_service_account", jiraAiServiceAccount),
			},
		},
	})
}

// An item that P0 already holds, such as one added in the P0 app, isn't staged again:
// P0 would accept the PUT, which returns an installed item to "stage". The apply fails,
// writes nothing, and says to import the item.
func TestJiraAiRefusesAnItemThatExists(t *testing.T) {
	cases := []struct {
		component, id, kind, config string
	}{
		{"connector", jiraAiConnector, "connector", jiraAiStaged(jiraAiGcp)},
		{"site", jiraAiSite, "site", jiraAiSiteBlock(`"prod"`)},
		{"pool-account", jiraAiPoolAccount, "agent account", strings.Replace(jiraAiPoolAccountBlock, "p0_jira_ai_site.example.id", `"acme"`, 1)},
	}
	for _, c := range cases {
		t.Run(c.component, func(t *testing.T) {
			f := newJiraAiFake(t)
			f.integrations["jira-ai"] = true
			f.items[fakeItemKey{"jira-ai", c.component, c.id}] = map[string]any{"label": c.id, "state": "installed"}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig(f.fakeP0) + c.config,
						ExpectError: jiraAiError(
							fmt.Sprintf("Jira %s already exists in P0", c.kind), fmt.Sprintf("P0 already has a Jira %s %q", c.kind, c.id),
							"terraform import <address> "+c.id,
						),
					},
				},
			})
			if f.writes != 0 {
				t.Errorf("P0 got %d writes; want none", f.writes)
			}
			if item := f.jiraItem(c.component, c.id); item["state"] != "installed" {
				t.Errorf("P0 item = %v; want it still installed", item)
			}
		})
	}
}

// P0 adds a site only for a connector that is installed. Naming one that is only
// staged fails the apply with P0's message and saves nothing, so the site is created
// once its connector is installed.
func TestJiraAiSiteNeedsAnInstalledConnector(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      providerConfig(f.fakeP0) + jiraAiStaged(jiraAiGcp) + jiraAiSiteBlock("p0_jira_ai_connector_staged.example.id"),
				ExpectError: jiraAiError("Could not stage site component", "The Jira connector prod, which the site acme uses, isn't installed"),
			},
			{
				PreConfig: func() {
					if item := f.jiraItem("site", jiraAiSite); item != nil {
						t.Errorf("P0 site after the refused stage = %v; want none", item)
					}
				},
				Config: jiraAiConnectorConfig(f, jiraAiGcp) + jiraAiSiteBlock("p0_jira_ai_connector.example.id"),
				Check:  resource.TestCheckResourceAttr("p0_jira_ai_site.example", "state", "installed"),
			},
		},
	})
}

// An agent account whose lookup fails is left staged in P0 and tainted in Terraform,
// with P0's message. The next apply replaces it: it removes the item, then stages and
// installs it.
func TestJiraAiPoolAccountFailedCheckIsReplaced(t *testing.T) {
	f := newJiraAiFake(t)
	f.failedLookup = "P0 couldn't find exactly one Atlassian account with this email on the site acme."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      jiraAiConfig(f),
				ExpectError: jiraAiError("Jira agent account install check failed", `the Jira agent account "agent-1"`, "couldn't find exactly one Atlassian account"),
			},
			{
				PreConfig: func() {
					if item := f.jiraItem("pool-account", jiraAiPoolAccount); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failedLookup = ""
				},
				Config: jiraAiConfig(f),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira_ai_pool_account.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "account_id", jiraAiAccountId),
				),
			},
		},
	})
}

// An item that P0 no longer reports as installed, such as one staged again outside
// Terraform, plans an update that installs it again, without staging it.
func TestJiraAiUnfinishedInstallConverges(t *testing.T) {
	f := newJiraAiFake(t)
	restage := func(component, id string) {
		f.update("jira-ai", component, id, func(item map[string]any) {
			item["state"] = "stage"
			delete(item, "accountId")
			if hosting, ok := item["hosting"].(map[string]any); ok {
				delete(hosting, "connectorServiceUri")
			}
		})
	}
	expect := func(updated string) resource.ConfigPlanChecks {
		checks := []plancheck.PlanCheck{}
		for _, address := range []string{"p0_jira_ai_connector_staged.example", "p0_jira_ai_connector.example", "p0_jira_ai_site.example", "p0_jira_ai_pool_account.example"} {
			action := plancheck.ResourceActionNoop
			if address == updated {
				action = plancheck.ResourceActionUpdate
			}
			checks = append(checks, plancheck.ExpectResourceAction(address, action))
		}
		return resource.ConfigPlanChecks{PreApply: checks}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraAiConfig(f)},
			{
				PreConfig:        func() { restage("connector", jiraAiConnector) },
				Config:           jiraAiConfig(f),
				ConfigPlanChecks: expect("p0_jira_ai_connector.example"),
				Check:            resource.TestCheckResourceAttr("p0_jira_ai_connector.example", "hosting.connector_service_uri", jiraAiServiceUri),
			},
			{
				PreConfig:        func() { restage("site", jiraAiSite) },
				Config:           jiraAiConfig(f),
				ConfigPlanChecks: expect("p0_jira_ai_site.example"),
				Check:            resource.TestCheckResourceAttr("p0_jira_ai_site.example", "state", "installed"),
			},
			{
				PreConfig:        func() { restage("pool-account", jiraAiPoolAccount) },
				Config:           jiraAiConfig(f),
				ConfigPlanChecks: expect("p0_jira_ai_pool_account.example"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.example", "account_id", jiraAiAccountId),
				),
			},
		},
	})
}

// An item removed in P0 is dropped from state, and the next apply creates it again.
func TestJiraAiSiteRemovedInP0(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraAiConfig(f)},
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.items, fakeItemKey{"jira-ai", "site", jiraAiSite})
					f.mu.Unlock()
				},
				Config: jiraAiConfig(f),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira_ai_site.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_jira_ai_site.example", "state", "installed"),
			},
		},
	})
}

// Values P0 would refuse fail the plan, in P0's words, before P0 is called.
func TestJiraAiValidation(t *testing.T) {
	staged := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_jira_ai_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}
	hosting := func(field, value string) string {
		fields := map[string]string{"type": "gcp", "project_id": jiraAiProject, "connector_name": jiraAiConnectorName, "connector_region": "us-central1"}
		fields[field] = value
		return fmt.Sprintf(`{
    type             = %q
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["type"], fields["project_id"], fields["connector_name"], fields["connector_region"])
	}
	site := func(field, value string) string {
		fields := map[string]string{
			"id": jiraAiSite, "connector": "prod", "site_url": "https://acme.atlassian.net", "vault_type": "gcp-sm", "project_id": "acme-secrets",
			"admin_client_id": "admin-client", "admin_secret_name": "jira-acme-admin",
		}
		fields[field] = value
		return fmt.Sprintf(`
resource "p0_jira_ai_site" "example" {
  id        = %q
  connector = %q
  site_url  = %q
  vault = {
    type       = %q
    project_id = %q
  }
  admin_client_id   = %q
  admin_secret_name = %q
}
`, fields["id"], fields["connector"], fields["site_url"], fields["vault_type"], fields["project_id"],
			fields["admin_client_id"], fields["admin_secret_name"])
	}
	pool := func(field, value string) string {
		fields := map[string]string{"id": jiraAiPoolAccount, "site": jiraAiSite, "email": "agent-1@serviceaccount.atlassian.com"}
		fields[field] = value
		return fmt.Sprintf(`
resource "p0_jira_ai_pool_account" "example" {
  id    = %q
  site  = %q
  email = %q
}
`, fields["id"], fields["site"], fields["email"])
	}

	for _, tc := range []struct {
		name, config, error string
	}{
		{"connector id with capitals", staged("Prod", jiraAiGcp), "lowercase letters"},
		{"AWS hosting", staged("prod", hosting("type", "aws")), "value must be one of"},
		{"project number", staged("prod", hosting("project_id", "123456789012")), "Enter the ID of the Google Cloud project"},
		{"connector name too long for a service account", staged("prod", hosting("connector_name", "p0-jira-connector-for-production")), "Use 6 to 30 characters"},
		{"zone in place of a region", staged("prod", hosting("connector_region", "us-central1-a")), "such as us-central1"},
		{"site id too long", site("id", strings.Repeat("a", 49)), "at most 48"},
		{"site URL with a path", site("site_url", "https://acme.atlassian.net/jira"), "with no path"},
		{"site URL off Atlassian", site("site_url", "https://jira.acme.com"), "atlassian.net"},
		{"AWS vault", site("vault_type", "aws-sm"), "value must be one of"},
		{"vault project number", site("project_id", "123456789012"), "Google Secret Manager installation"},
		{"secret's resource name", site("admin_secret_name", "projects/acme-secrets/secrets/jira-acme-admin"), "isn't a secret's name"},
		{"blank admin client", site("admin_client_id", " "), "admin OAuth client's ID"},
		{"agent account site too long", pool("site", strings.Repeat("a", 49)), "at most 48"},
		{"blank email", pool("email", ""), "service account's email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newJiraAiFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: jiraAiError(tc.error),
					},
				},
			})
		})
	}
}

// An import refuses an ID that P0's rule refuses, before it reads anything.
func TestJiraAiImportRefusesAnInvalidId(t *testing.T) {
	f := newJiraAiFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        providerConfig(f.fakeP0) + jiraAiSiteBlock(`"prod"`),
				ResourceName:  "p0_jira_ai_site.example",
				ImportState:   true,
				ImportStateId: "acme/../connector/prod",
				ExpectError:   jiraAiError("Invalid Jira site ID", "up to 48 characters"),
			},
		},
	})
}

// The variables that name what TestAccJiraAi installs: a Jira connector already
// deployed on Cloud Run where they say, running as the service account P0 names after
// it, a Jira Cloud site whose admin client's secret the connector can read, and an
// Atlassian service account on it. P0 checks them through the connector and
// Atlassian, so they must be real.
func jiraAiAccEnv(install *jiraAiAccInstall) map[string]*string {
	return map[string]*string{
		"P0_JIRA_AI_PROJECT_ID":        &install.projectId,
		"P0_JIRA_AI_CONNECTOR_NAME":    &install.connectorName,
		"P0_JIRA_AI_CONNECTOR_REGION":  &install.connectorRegion,
		"P0_JIRA_AI_SITE_URL":          &install.siteUrl,
		"P0_JIRA_AI_VAULT_PROJECT_ID":  &install.vaultProjectId,
		"P0_JIRA_AI_ADMIN_CLIENT_ID":   &install.adminClientId,
		"P0_JIRA_AI_ADMIN_SECRET_NAME": &install.adminSecretName,
		"P0_JIRA_AI_AGENT_EMAIL":       &install.agentEmail,
	}
}

type jiraAiAccInstall struct {
	projectId, connectorName, connectorRegion               string
	siteUrl, vaultProjectId, adminClientId, adminSecretName string
	agentEmail                                              string
}

// TestAccJiraAi installs into the P0 organization P0_ORG, served at P0_HOST (the
// provider's default host unless set), with the API token P0_API_TOKEN. Copies of
// onepassword_test.go's helpers, so the two tests don't depend on each other.
const (
	jiraAiAccOrgEnv      = "P0_ORG"
	jiraAiAccHostEnv     = "P0_HOST"
	jiraAiAccTokenEnv    = "P0_API_TOKEN"
	jiraAiAccDefaultHost = "https://api.p0.app"
)

func jiraAiAccHost() string {
	if host := os.Getenv(jiraAiAccHostEnv); host != "" {
		return host
	}
	return jiraAiAccDefaultHost
}

// Calls the acceptance-test organization's API directly, to check that a destroyed
// install is gone.
func jiraAiAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        fmt.Sprintf("%s/o/%s", jiraAiAccHost(), os.Getenv(jiraAiAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(jiraAiAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// Skips TestAccJiraAi unless it has a P0 organization to run against (P0_ORG and
// P0_API_TOKEN, and P0_HOST unless it's the default) and every P0_JIRA_AI_* variable
// is set. It skips rather than fails, because CI sets TF_ACC without an organization.
func testAccJiraAiPreCheck(t *testing.T) {
	if os.Getenv(jiraAiAccOrgEnv) == "" || os.Getenv(jiraAiAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the Jira (AI) acceptance tests against a P0 organization", jiraAiAccOrgEnv, jiraAiAccTokenEnv)
	}
	var missing []string
	for name := range jiraAiAccEnv(&jiraAiAccInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the Jira (AI) acceptance tests", strings.Join(missing, ", "))
	}
}

// The chain the examples install, for what the P0_JIRA_AI_* variables name, with the
// connector already deployed. The provider reads the API token from P0_API_TOKEN
// itself.
func jiraAiAccConfig() string {
	var install jiraAiAccInstall
	for name, value := range jiraAiAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}

resource "p0_jira_ai_connector_staged" "test" {
  id = "tf-acc"
  hosting = {
    type             = "gcp"
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }
}

resource "p0_jira_ai_connector" "test" {
  id = p0_jira_ai_connector_staged.test.id
  hosting = {
    type             = p0_jira_ai_connector_staged.test.hosting.type
    project_id       = p0_jira_ai_connector_staged.test.hosting.project_id
    connector_name   = p0_jira_ai_connector_staged.test.hosting.connector_name
    connector_region = p0_jira_ai_connector_staged.test.hosting.connector_region
  }
}

resource "p0_jira_ai_site" "test" {
  id        = "tf-acc"
  connector = p0_jira_ai_connector.test.id
  site_url  = %q
  vault = {
    type       = "gcp-sm"
    project_id = %q
  }
  admin_client_id   = %q
  admin_secret_name = %q
}

resource "p0_jira_ai_pool_account" "test" {
  id    = "tf-acc"
  site  = p0_jira_ai_site.test.id
  email = %q
}
`, os.Getenv(jiraAiAccOrgEnv), jiraAiAccHost(),
		install.projectId, install.connectorName, install.connectorRegion,
		install.siteUrl, install.vaultProjectId, install.adminClientId, install.adminSecretName,
		install.agentEmail)
}

// Checks that P0 no longer has the item behind any destroyed Jira (AI) resource. The
// staged connector's item outlives the final connector's rollback, so it's checked
// last of all, once every resource is destroyed.
func checkJiraAiAccDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	components := map[string]string{
		"p0_jira_ai_connector_staged": "connector",
		"p0_jira_ai_connector":        "connector",
		"p0_jira_ai_site":             "site",
		"p0_jira_ai_pool_account":     "pool-account",
	}
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := components[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(fmt.Sprintf("integrations/jira-ai/config/%s/%s", component, rs.Primary.ID), &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the Jira (AI) %s %s was deleted: %w", component, rs.Primary.ID, err)
			}
			return fmt.Errorf("the Jira (AI) %s %s is still in P0", component, rs.Primary.ID)
		}
		return nil
	}
}

// TestAccJiraAi installs a connector, a site and an agent account in a P0 organization
// (see testAccJiraAiPreCheck), checks that a second plan is empty, imports the agent
// account, and destroys them all. The organization must run a P0 version with the
// Jira (AI) integration (app#7540).
func TestAccJiraAi(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccJiraAiPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAiAccDestroyed(jiraAiAccClient()),
		Steps: []resource.TestStep{
			{
				Config: jiraAiAccConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira_ai_connector.test", "state", "installed"),
					resource.TestCheckResourceAttrSet("p0_jira_ai_connector.test", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_jira_ai_site.test", "state", "installed"),
					resource.TestCheckResourceAttrSet("p0_jira_ai_site.test", "cloud_id"),
					resource.TestCheckResourceAttr("p0_jira_ai_pool_account.test", "state", "installed"),
					resource.TestCheckResourceAttrSet("p0_jira_ai_pool_account.test", "account_id"),
				),
			},
			{
				ResourceName:      "p0_jira_ai_pool_account.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
