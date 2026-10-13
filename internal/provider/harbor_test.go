package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	installharbor "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/harbor"
)

const (
	harborConnector      = "prod"
	harborConnectorName  = "p0-harbor-prod"
	harborProject        = "acme-prod"
	harborServiceAccount = harborConnectorName + "@" + harborProject + ".iam.gserviceaccount.com"
	harborServiceUri     = "https://" + harborConnectorName + "-abc123-uw.a.run.app"
)

// Where a Harbor connector runs, as a customer's Terraform sets it, with one field
// set to value.
func harborGcpHosting(field, value string) string {
	fields := map[string]string{"project_id": harborProject, "connector_name": harborConnectorName, "connector_region": "us-west1"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "gcp"
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["project_id"], fields["connector_name"], fields["connector_region"])
}

func harborAwsHosting(field, value string) string {
	fields := map[string]string{"account_id": "123456789012", "connector_name": harborConnectorName, "connector_region": "us-west-2"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
}

var (
	harborGcp = harborGcpHosting("project_id", harborProject)
	harborAws = harborAwsHosting("connector_name", harborConnectorName)
)

func harborStaged(id, hosting string) string {
	return fmt.Sprintf(`
resource "p0_harbor_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
}

func harborStagedConfig(f *harborFake, hosting string) string {
	return providerConfig(f.fakeP0) + harborStaged(harborConnector, hosting)
}

// A connector staged at one hosting, and installed at another. A customer's Terraform
// gives both the same one. Each resource writes its hosting out, and they are chained
// through `id`: Terraform 1.0 and 1.1 can't read a nested attribute of a resource that
// isn't created yet, and no version lets the final resource take the staged one's
// hosting whole, since that sets its read-only attributes.
func harborMovedConfig(f *harborFake, staged, final string) string {
	return harborStagedConfig(f, staged) + fmt.Sprintf(`
resource "p0_harbor_connector" "example" {
  id      = p0_harbor_connector_staged.example.id
  hosting = %s
}
`, final)
}

func harborConnectorConfig(f *harborFake, hosting string) string {
	return harborMovedConfig(f, hosting, hosting)
}

// Behaves as P0's Harbor installers do for a connector:
//   - staging one on Google Cloud generates the service account it must run as
//   - verifying one on Google Cloud resolves its Cloud Run URL
//   - a verify or configure that would move a connector from where it was staged is
//     refused
//
// The fields make it fail, for the test of each failure.
type harborFake struct {
	*fakeP0
	// Harbor's feature flag is off, so P0 answers as if it had no Harbor items.
	disabled bool
	// The service account P0 generates for a connector on Google Cloud, or nil for none.
	serviceAccount func(hosting map[string]any) any
	// P0's message for a connector that fails its install check, on verify.
	failedCheck string
	// P0's message for a connector it refuses to stage.
	stageRefusal string
	// How many writes to Harbor items P0 got, refused or not.
	writes int
}

// The fields of a connector's hosting that P0 fixes when it creates the item.
var harborAddressFields = []string{"type", "accountId", "projectId", "connectorName", "connectorRegion"}

func newHarborFake(t *testing.T) *harborFake {
	f := &harborFake{fakeP0: newFakeP0(t)}
	f.serviceAccount = func(hosting map[string]any) any {
		return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", hosting["connectorName"], hosting["projectId"])
	}

	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "harbor" {
			return 0, ""
		}
		f.writes++
		if f.disabled {
			return http.StatusNotFound, "Not found"
		}

		hosting, _ := step.body["hosting"].(map[string]any)
		if step.nextState == "stage" {
			if f.stageRefusal != "" {
				return http.StatusBadRequest, f.stageRefusal
			}
			// P0 takes a PUT over a connector it holds only when the hosting matches.
			if stored, ok := step.previous["hosting"].(map[string]any); ok {
				for _, field := range harborAddressFields {
					if hosting[field] != stored[field] {
						return http.StatusConflict, "This Harbor connector already exists with another hosting."
					}
				}
			}
			return 0, ""
		}

		staged, _ := step.previous["hosting"].(map[string]any)
		for _, field := range harborAddressFields {
			if sent, ok := hosting[field]; ok && sent != staged[field] {
				return http.StatusUnprocessableEntity, fmt.Sprintf(
					"'hosting/%s/%s' can only be altered on initial installation. Create a new installation to change this field.",
					staged["type"], field,
				)
			}
		}
		if step.nextState == "configure" && f.failedCheck != "" {
			return http.StatusUnprocessableEntity, f.failedCheck
		}
		return 0, ""
	}

	f.normalize = func(key fakeItemKey, item map[string]any) {
		hosting, ok := item["hosting"].(map[string]any)
		if key.integration != "harbor" || key.component != "connector" || !ok || hosting["type"] != "gcp" {
			return
		}
		if account := f.serviceAccount(hosting); account != nil {
			hosting["connectorServiceAccount"] = account
		}
		if item["state"] != "stage" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	}

	return f
}

func (f *harborFake) connector() map[string]any {
	return f.item("harbor", "connector", harborConnector)
}

// The connector's item in P0, if it is at state and holds exactly the hosting want.
func (f *harborFake) checkConnector(state string, want map[string]any) error {
	item := f.connector()
	hosting, _ := item["hosting"].(map[string]any)
	if item["state"] != state || len(hosting) != len(want) {
		return fmt.Errorf("P0 item = %v; want it at %s with the hosting %v", item, state, want)
	}
	for field, value := range want {
		if hosting[field] != value {
			return fmt.Errorf("P0 item = %v; want it at %s with the hosting %v", item, state, want)
		}
	}
	return nil
}

func checkHarborDestroyed(f *harborFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		return nil
	}
}

// Terraform wraps a diagnostic's text, so the words of each part of an error may be on
// different lines. The parts match in order, with anything between them.
func harborError(parts ...string) *regexp.Regexp {
	patterns := []string{}
	for _, part := range parts {
		patterns = append(patterns, strings.Join(strings.Fields(regexp.QuoteMeta(part)), `\s+`))
	}
	return regexp.MustCompile(`(?s)` + strings.Join(patterns, `.*`))
}

// The connector on Google Cloud: staging it reads the service account that P0
// generates, and installing it reads the Cloud Run URL that P0 resolves.
func TestHarborConnectorOnGoogleCloud(t *testing.T) {
	f := newHarborFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: harborConnectorConfig(f, harborGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_harbor_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_harbor_connector_staged.example", "hosting.connector_service_account", harborServiceAccount),
					resource.TestCheckNoResourceAttr("p0_harbor_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "hosting.project_id", harborProject),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "hosting.connector_service_account", harborServiceAccount),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "hosting.connector_service_uri", harborServiceUri),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":                    "gcp",
							"projectId":               harborProject,
							"connectorName":           harborConnectorName,
							"connectorRegion":         "us-west1",
							"connectorServiceAccount": harborServiceAccount,
							"connectorServiceUri":     harborServiceUri,
						})
					},
				),
			},
			{
				ResourceName:      "p0_harbor_connector_staged.example",
				ImportState:       true,
				ImportStateId:     harborConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state", "hosting.connector_service_uri"},
			},
			{
				ResourceName:      "p0_harbor_connector.example",
				ImportState:       true,
				ImportStateId:     harborConnector,
				ImportStateVerify: true,
			},
			// The ID goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  "p0_harbor_connector.example",
				ImportState:   true,
				ImportStateId: "prod/other",
				ExpectError:   harborError("Invalid Harbor connector ID", "Use lowercase letters, digits and hyphens"),
			},
		},
	})
}

// The connector on AWS, where P0 generates nothing: it derives the function's address
// from the hosting.
func TestHarborConnectorOnAws(t *testing.T) {
	f := newHarborFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: harborConnectorConfig(f, harborAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_harbor_connector_staged.example", "state", "stage"),
					resource.TestCheckNoResourceAttr("p0_harbor_connector_staged.example", "hosting.connector_service_account"),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_harbor_connector.example", "hosting.connector_service_account"),
					resource.TestCheckNoResourceAttr("p0_harbor_connector.example", "hosting.connector_service_uri"),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":            "aws",
							"accountId":       "123456789012",
							"connectorName":   harborConnectorName,
							"connectorRegion": "us-west-2",
						})
					},
				),
			},
			{
				ResourceName:      "p0_harbor_connector.example",
				ImportState:       true,
				ImportStateId:     harborConnector,
				ImportStateVerify: true,
			},
		},
	})
}

// A hosting that comes from a variable is unknown while Terraform validates the
// configuration, so the resources' checks skip it then, and known when it plans.
func TestHarborConnectorHostingFromVariable(t *testing.T) {
	f := newHarborFake(t)
	config := providerConfig(f.fakeP0) + fmt.Sprintf(`
variable "hosting" {
  type = object({
    type             = string
    project_id       = string
    connector_name   = string
    connector_region = string
  })
  default = %s
}

resource "p0_harbor_connector_staged" "example" {
  id      = %q
  hosting = var.hosting
}

resource "p0_harbor_connector" "example" {
  id      = p0_harbor_connector_staged.example.id
  hosting = var.hosting
}
`, harborGcp, harborConnector)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_harbor_connector_staged.example", "hosting.connector_service_account", harborServiceAccount),
					resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
				),
			},
		},
	})
}

// Where a connector runs is fixed when it is staged, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestHarborConnectorHostingReplacesBoth(t *testing.T) {
	f := newHarborFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{Config: harborConnectorConfig(f, harborGcp)},
			{
				Config: harborConnectorConfig(f, harborGcpHosting("connector_region", "europe-west1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_harbor_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					return f.checkConnector("installed", map[string]any{
						"type":                    "gcp",
						"projectId":               harborProject,
						"connectorName":           harborConnectorName,
						"connectorRegion":         "europe-west1",
						"connectorServiceAccount": harborServiceAccount,
						"connectorServiceUri":     harborServiceUri,
					})
				},
			},
		},
	})
}

// Destroying only the final resource returns its item to "stage", so the staged
// resource still owns it, and a later apply installs it again.
func TestHarborConnectorRollback(t *testing.T) {
	f := newHarborFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{Config: harborConnectorConfig(f, harborAws)},
			{
				Config: harborStagedConfig(f, harborAws),
				Check: func(*terraform.State) error {
					return f.checkConnector("stage", map[string]any{
						"type":            "aws",
						"accountId":       "123456789012",
						"connectorName":   harborConnectorName,
						"connectorRegion": "us-west-2",
					})
				},
			},
			{
				Config: harborConnectorConfig(f, harborAws),
				Check:  resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
			},
		},
	})
}

// The final resource never stages the item, which would move the connector. It only
// verifies it, so P0 refuses a hosting that isn't the staged one and the connector
// stays where it was staged.
func TestHarborConnectorMustMatchStaged(t *testing.T) {
	f := newHarborFake(t)
	stagedAt := map[string]any{
		"type":                    "gcp",
		"projectId":               harborProject,
		"connectorName":           harborConnectorName,
		"connectorRegion":         "us-west1",
		"connectorServiceAccount": harborServiceAccount,
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: harborMovedConfig(f, harborGcp, harborGcpHosting("connector_region", "europe-west1")),
				ExpectError: harborError(
					"Harbor connector install check failed",
					"'hosting/gcp/connectorRegion' can only be altered on initial installation",
				),
			},
			{
				PreConfig: func() {
					if err := f.checkConnector("stage", stagedAt); err != nil {
						t.Errorf("after the refused install: %v", err)
					}
				},
				Config: harborConnectorConfig(f, harborGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_harbor_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
			},
		},
	})
}

// A connector that fails P0's install check fails the apply with P0's message, and
// stays staged. The next apply checks it again.
func TestHarborConnectorFailedCheck(t *testing.T) {
	f := newHarborFake(t)
	f.failedCheck = "P0 couldn't find the Cloud Run service p0-harbor-prod."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      harborConnectorConfig(f, harborGcp),
				ExpectError: harborError("Harbor connector install check failed", `Harbor connector "prod"`, f.failedCheck),
			},
			{
				PreConfig: func() {
					if item := f.connector(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failedCheck = ""
				},
				Config: harborConnectorConfig(f, harborGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_harbor_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
			},
		},
	})
}

// On Google Cloud the connector can't be deployed without the service account P0
// generates, so a connector that comes back without one fails the apply and says so.
// On AWS there is none to miss.
func TestHarborConnectorMissingServiceAccount(t *testing.T) {
	missing := harborError("Bad API response", "Harbor connector prod is hosted on Google Cloud, but P0 returned no service account for it")
	none := func(map[string]any) any { return nil }

	for name, account := range map[string]func(map[string]any) any{
		"no account":    none,
		"blank account": func(map[string]any) any { return "" },
	} {
		// The connector P0 staged isn't left behind, so once P0 returns the account the
		// next apply stages it.
		t.Run(name+" when staged on Google Cloud", func(t *testing.T) {
			f := newHarborFake(t)
			working := f.serviceAccount
			f.serviceAccount = account
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkHarborDestroyed(f),
				Steps: []resource.TestStep{
					{Config: harborStagedConfig(f, harborGcp), ExpectError: missing},
					{
						PreConfig: func() {
							if item := f.connector(); item != nil {
								t.Errorf("P0 item after the failed staging = %v; want none", item)
							}
							f.serviceAccount = working
						},
						Config: harborStagedConfig(f, harborGcp),
						Check:  resource.TestCheckResourceAttr("p0_harbor_connector_staged.example", "hosting.connector_service_account", harborServiceAccount),
					},
				},
			})
		})
	}

	// The account goes missing after the connector is installed, as it would for an
	// item that someone changed in P0.
	t.Run("no account when read on Google Cloud", func(t *testing.T) {
		f := newHarborFake(t)
		setAccount := func(account any) {
			f.update("harbor", "connector", harborConnector, func(item map[string]any) {
				hosting, ok := item["hosting"].(map[string]any)
				switch {
				case !ok:
					t.Errorf("P0 item = %v; want one with a hosting", item)
				case account == nil:
					delete(hosting, "connectorServiceAccount")
				default:
					hosting["connectorServiceAccount"] = account
				}
			})
		}

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborDestroyed(f),
			Steps: []resource.TestStep{
				{Config: harborConnectorConfig(f, harborGcp)},
				{
					PreConfig: func() { setAccount(nil) },
					Config:    harborConnectorConfig(f, harborGcp),
					PlanOnly:  true,
					ExpectError: harborError(
						"Bad API response", "Harbor connector prod is hosted on Google Cloud, but P0 returned no service account for it",
						"plan and apply other changes with `-refresh=false`", "terraform state rm",
					),
				},
				// With the account back, the resources read again, and can be destroyed.
				{
					PreConfig: func() { setAccount(harborServiceAccount) },
					Config:    harborConnectorConfig(f, harborGcp),
				},
			},
		})
	})

	t.Run("no account on AWS", func(t *testing.T) {
		f := newHarborFake(t)
		f.serviceAccount = none
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborDestroyed(f),
			Steps: []resource.TestStep{
				{
					Config: harborConnectorConfig(f, harborAws),
					Check:  resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
				},
			},
		})
	})
}

// A connector that P0 already holds, such as one added in the P0 app, isn't staged
// again: P0 would accept the PUT, which returns an installed connector to "stage" and
// stops every instance behind it. The apply fails, writes nothing, and says to import
// the connector. So does one whose read fails for another reason.
func TestHarborConnectorStagedOverExisting(t *testing.T) {
	key := fakeItemKey{"harbor", "connector", harborConnector}
	for _, state := range []string{"stage", "installed"} {
		t.Run(state, func(t *testing.T) {
			f := newHarborFake(t)
			f.integrations["harbor"] = true
			f.items[key] = map[string]any{
				"hosting": map[string]any{
					"type": "gcp", "projectId": harborProject, "connectorName": harborConnectorName, "connectorRegion": "us-west1",
					"connectorServiceAccount": harborServiceAccount,
				},
				"label": harborConnector, "state": state,
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: harborStagedConfig(f, harborGcp),
						ExpectError: harborError(
							"Harbor connector already exists in P0", `P0 already has a Harbor connector "prod"`,
							"terraform import <address> prod",
						),
					},
				},
			})
			if f.writes != 0 {
				t.Errorf("P0 got %d writes; want none", f.writes)
			}
			if item := f.connector(); item["state"] != state {
				t.Errorf("P0 item = %v; want it still at %s", item, state)
			}
		})
	}
}

// With Harbor's feature flag off, P0 answers the request that stages a connector with
// a bare 404. The apply fails with what that means, and what to do about it.
func TestHarborConnectorUnavailable(t *testing.T) {
	f := newHarborFake(t)
	f.disabled = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: harborStagedConfig(f, harborGcp),
				ExpectError: harborError(
					"Harbor isn't enabled for this P0 organization",
					`P0 couldn't stage or install the Harbor connector "prod"`,
					"contact support@p0.dev to have it enabled",
					"P0's response: 404 Not Found: Not found",
				),
			},
		},
	})
}

// While Harbor's feature flag is off, P0 answers a read with a bare 404 too. The
// refresh fails and keeps both resources, rather than drop them and have the next
// apply stage the installed connector again. Once the flag is back on, nothing has
// changed.
func TestHarborConnectorNotFoundOnRead(t *testing.T) {
	f := newHarborFake(t)
	key := fakeItemKey{"harbor", "connector", harborConnector}
	// The fake has no flag, so the item is taken out while the flag is "off", and put
	// back as it was: P0 keeps an item while it hides it.
	var hidden map[string]any
	hide := func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		hidden = f.items[key]
		delete(f.items, key)
	}
	unhide := func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.items[key] = hidden
	}
	// Each resource fails on its own: Terraform shows the resource after the summary.
	notFound := func(address string) string {
		return `Harbor connector not found in P0\s+with ` + regexp.QuoteMeta(address) + `,` +
			`.*P0 has no Harbor connector "prod".*-refresh=false.*terraform\s+state\s+rm\s+<address>.*404 Not Found`
	}
	staged, final := notFound("p0_harbor_connector_staged.example"), notFound("p0_harbor_connector.example")
	notFoundForBoth := regexp.MustCompile(`(?s)(` + staged + `.*` + final + `|` + final + `.*` + staged + `)`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{Config: harborConnectorConfig(f, harborGcp)},
			{
				PreConfig: hide,
				// The final resource names the connector itself, so that Terraform refreshes
				// it whether or not the staged resource fails.
				Config: harborStagedConfig(f, harborGcp) + fmt.Sprintf(`
resource "p0_harbor_connector" "example" {
  id      = %q
  hosting = %s
}
`, harborConnector, harborGcp),
				PlanOnly:    true,
				ExpectError: notFoundForBoth,
			},
			{
				PreConfig: unhide,
				Config:    harborConnectorConfig(f, harborGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_harbor_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionNoop),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.connector(); item["state"] != "installed" {
						return fmt.Errorf("P0 item = %v; want it still installed", item)
					}
					return nil
				},
			},
		},
	})
}

// Other resources keep the framework's usual behaviour: an item that P0 no longer has
// is dropped from state, and the next apply creates it again.
func TestNonHarborResourceDroppedOnNotFound(t *testing.T) {
	f := newGoogleDriveFake(t)
	config := providerConfig(f) + fmt.Sprintf(`
resource "p0_google_drive_connector_staged" "example" {
  project_id = %q
}
`, driveProject)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.items, fakeItemKey{"google-drive-ai", "connector", driveProject})
					f.mu.Unlock()
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_google_drive_connector_staged.example", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// The flag can go off between staging and installing. The verify step gets the same
// bare 404, and fails with the same diagnostic.
func TestHarborConnectorUnavailableOnVerify(t *testing.T) {
	f := newHarborFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{Config: harborStagedConfig(f, harborGcp)},
			{
				PreConfig: func() { f.disabled = true },
				Config:    harborConnectorConfig(f, harborGcp),
				ExpectError: harborError(
					"Harbor isn't enabled for this P0 organization",
					`P0 couldn't stage or install the Harbor connector "prod"`,
					"P0's response: 404 Not Found: Not found",
				),
			},
			// With the flag back on, the next apply installs it.
			{
				PreConfig: func() { f.disabled = false },
				Config:    harborConnectorConfig(f, harborGcp),
				Check:     resource.TestCheckResourceAttr("p0_harbor_connector.example", "state", "installed"),
			},
		},
	})
}

// P0 has the last word on a value that the plan let through: its refusal fails the
// apply in its own words, and stages nothing.
func TestHarborConnectorRefusedByP0(t *testing.T) {
	f := newHarborFake(t)
	f.stageRefusal = "P0 couldn't use this connector name."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      harborStagedConfig(f, harborGcp),
				ExpectError: harborError("Could not stage connector component", "400 Bad Request: P0 couldn't use this connector name."),
			},
		},
	})
}

func TestHarborConnectorValidation(t *testing.T) {
	final := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_harbor_connector" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
	}
	const invalidId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters"
	const awsName = "Use 2 to 64 characters for the connector name"
	const gcpName = "Use 6 to 30 characters for the connector name"

	for _, tc := range []struct {
		name, config string
		errors       []string
	}{
		{"id with capitals", harborStaged("Prod", harborGcp), []string{invalidId, "got: Prod"}},
		{"id ending with a hyphen", harborStaged("prod-", harborGcp), []string{invalidId}},
		{"id with a path", harborStaged("prod/other", harborGcp), []string{invalidId}},
		{"id over 63 characters", harborStaged(strings.Repeat("a", 64), harborGcp), []string{"string length must be at most 63"}},
		{"final id with capitals", final("Prod", harborGcp), []string{invalidId}},
		{"AWS name ECR wouldn't take", harborStaged("prod", harborAwsHosting("connector_name", "p0--harbor")), []string{"Invalid Harbor connector name", awsName, `Got: "p0--harbor"`}},
		{"AWS name over 64 characters", harborStaged("prod", harborAwsHosting("connector_name", "p"+strings.Repeat("0", 64))), []string{awsName}},
		{"AWS name with capitals", harborStaged("prod", harborAwsHosting("connector_name", "P0-Harbor")), []string{awsName}},
		{"Google Cloud name too long for a service account", harborStaged("prod", harborGcpHosting("connector_name", "p0-harbor-connector-for-production")), []string{gcpName}},
		{"Google Cloud name too short for a service account", harborStaged("prod", harborGcpHosting("connector_name", "p0-hb")), []string{gcpName}},
		// P0 would trim the name and store it, and the apply would then fail on a name
		// that reads back different.
		{"name with whitespace around it", harborStaged("prod", harborGcpHosting("connector_name", harborConnectorName+" ")), []string{gcpName, `Got: "p0-harbor-prod "`}},
		{"final name too long for a service account", final("prod", harborGcpHosting("connector_name", "p0-harbor-connector-for-production")), []string{gcpName}},
		{
			"project on AWS hosting",
			harborStaged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-west-2"
  }`),
			[]string{"Missing AWS account ID"},
		},
		{
			"account on Google Cloud hosting",
			final("prod", `{
    type             = "gcp"
    account_id       = "123456789012"
    project_id       = "acme-prod"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-west1"
  }`),
			[]string{"Unexpected AWS account ID"},
		},
		{"project with an underscore", harborStaged("prod", harborGcpHosting("project_id", "acme_prod")), []string{"Google Cloud project identifiers must consist of alphanumeric characters and hyphens only"}},
		{"account that isn't one", harborStaged("prod", harborAwsHosting("account_id", "1234")), []string{"12 numeric digits"}},
		{"region with whitespace around it", harborStaged("prod", harborAwsHosting("connector_region", " us-west-2")), []string{"valid AWS or Google Cloud region"}},
		{
			"hosting on another cloud",
			harborStaged("prod", `{
    type             = "azure"
    connector_name   = "p0-harbor-prod"
    connector_region = "westus2"
  }`),
			[]string{`value must be one of: ["aws" "gcp"]`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: harborError(tc.errors...),
					},
				},
			})
		})
	}
}

// Terraform never plans an update of the staged resource, since each attribute that
// can be set requires replacement. One that reaches the provider anyway fails, rather
// than report a change it didn't make, or stage the connector again.
func TestHarborConnectorStagedUpdateFails(t *testing.T) {
	var resp fwresource.UpdateResponse
	installharbor.NewHarborConnectorStaged().Update(context.Background(), fwresource.UpdateRequest{}, &resp)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Please report this as a provider bug") {
		t.Errorf("diagnostics = %v; want one error asking for a bug report", resp.Diagnostics)
	}
}
