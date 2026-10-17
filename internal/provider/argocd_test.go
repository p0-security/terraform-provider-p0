package provider

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	installargocd "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/argocd"
)

// copy of internal/provider/harbor_test.go, with the Argo CD instance's managing_app
// and namespace (INT-576).

const (
	argocdConnector      = "prod"
	argocdConnectorName  = "p0-argocd-prod"
	argocdProject        = "acme-prod"
	argocdServiceAccount = argocdConnectorName + "@" + argocdProject + ".iam.gserviceaccount.com"
	argocdServiceUri     = "https://" + argocdConnectorName + "-abc123-uw.a.run.app"
)

// Where an Argo CD connector runs, as a customer's Terraform sets it, with one field
// set to value.
func argocdGcpHosting(field, value string) string {
	fields := map[string]string{"project_id": argocdProject, "connector_name": argocdConnectorName, "connector_region": "us-west1"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "gcp"
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["project_id"], fields["connector_name"], fields["connector_region"])
}

func argocdAwsHosting(field, value string) string {
	fields := map[string]string{"account_id": "123456789012", "connector_name": argocdConnectorName, "connector_region": "us-west-2"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
}

var (
	argocdGcp = argocdGcpHosting("project_id", argocdProject)
	argocdAws = argocdAwsHosting("connector_name", argocdConnectorName)
)

func argocdStaged(id, hosting string) string {
	return fmt.Sprintf(`
resource "p0_argocd_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
}

func argocdStagedConfig(f *argocdFake, hosting string) string {
	return providerConfig(f.fakeP0) + argocdStaged(argocdConnector, hosting)
}

// A connector staged at one hosting, and installed at another. A customer's Terraform
// gives both the same one. Each resource writes its hosting out, and they are chained
// through `id`: Terraform 1.0 and 1.1 can't read a nested attribute of a resource that
// isn't created yet, and no version lets the final resource take the staged one's
// hosting whole, since that sets its read-only attributes.
func argocdMovedConfig(f *argocdFake, staged, final string) string {
	return argocdStagedConfig(f, staged) + fmt.Sprintf(`
resource "p0_argocd_connector" "example" {
  id      = p0_argocd_connector_staged.example.id
  hosting = %s
}
`, final)
}

func argocdConnectorConfig(f *argocdFake, hosting string) string {
	return argocdMovedConfig(f, hosting, hosting)
}

// Behaves as P0's Argo CD installers do for a connector:
//   - staging one on Google Cloud generates the service account it must run as
//   - verifying one on Google Cloud resolves its Cloud Run URL
//   - a verify or configure that would move a connector from where it was staged is
//     refused
//
// and for an instance, as refuseInstance says. The fields make it fail, for the test
// of each failure.
type argocdFake struct {
	*fakeP0
	// The service account P0 generates for a connector on Google Cloud, or nil for none.
	serviceAccount func(hosting map[string]any) any
	// P0's message for a connector that fails its install check, on verify.
	failedCheck string
	// P0's message for a connector it refuses to stage.
	stageRefusal string
	// P0's message for an instance that fails its install check, on verify.
	failedInstanceCheck string
	// The instance's check couldn't reach Argo CD, so P0 answers 502 rather than 422.
	instanceCheckUnreachable bool
	// The state each write to an instance would move it to, in order, refused or not.
	instanceSteps []string
	// How many writes to Argo CD items P0 got, refused or not.
	writes int
}

// The IDs of the Argo CD instances that name the connector id, sorted.
func argocdInstancesUsing(items map[fakeItemKey]map[string]any, id string) []string {
	users := []string{}
	for key, item := range items {
		if key.integration == "argocd" && key.component == "argocd-instance" && item["connector"] == id {
			users = append(users, key.id)
		}
	}
	slices.Sort(users)
	return users
}

// P0's refusal to stage or verify again a connector that instances use.
func argocdConnectorInUse(id string, action string, users []string) string {
	return fmt.Sprintf("The Argo CD connector %s can't be %s while an Argo CD instance uses it (%s). Remove each instance that uses it first.",
		id, action, strings.Join(users, ", "))
}

// The fields of a connector's hosting that P0 fixes when it creates the item.
var argocdAddressFields = []string{"type", "accountId", "projectId", "connectorName", "connectorRegion"}

func newArgocdFake(t *testing.T) *argocdFake {
	f := &argocdFake{fakeP0: newFakeP0(t)}
	f.serviceAccount = func(hosting map[string]any) any {
		return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", hosting["connectorName"], hosting["projectId"])
	}

	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "argocd" {
			return 0, ""
		}
		f.writes++
		if step.key.component == "argocd-instance" {
			return f.refuseInstance(step, items)
		}

		hosting, _ := step.body["hosting"].(map[string]any)
		if step.nextState == "stage" {
			if f.stageRefusal != "" {
				return http.StatusBadRequest, f.stageRefusal
			}
			// P0 takes a PUT over a connector it holds only when the hosting matches, and
			// no instance uses the connector.
			if stored, ok := step.previous["hosting"].(map[string]any); ok {
				for _, field := range argocdAddressFields {
					if hosting[field] != stored[field] {
						return http.StatusConflict, fmt.Sprintf("An Argo CD connector with the ID %s already exists, and a connector can't be changed once it's added. "+
							"To run a connector somewhere else, add one under another ID.", step.key.id)
					}
				}
				if users := argocdInstancesUsing(items, step.key.id); len(users) > 0 {
					return http.StatusConflict, argocdConnectorInUse(step.key.id, "staged again", users)
				}
			}
			return 0, ""
		}

		if users := argocdInstancesUsing(items, step.key.id); step.nextState == "configure" && step.previous["state"] == "installed" && len(users) > 0 {
			return http.StatusConflict, argocdConnectorInUse(step.key.id, "verified again", users)
		}

		staged, _ := step.previous["hosting"].(map[string]any)
		for _, field := range argocdAddressFields {
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
		if key.integration != "argocd" || key.component != "connector" || !ok || hosting["type"] != "gcp" {
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

// The cloud each vault type is in, as a connector's hosting names it.
var argocdVaultCloud = map[string]string{"aws-sm": "aws", "gcp-sm": "gcp"}

// Answers as P0's installer for an instance is to: the Argo CD backend (INT-571) isn't
// written yet, so this is Harbor's installer (app#7443, INT-394, at 5341116959), with
// the managing Application and namespace fixed like the URL, in its words. The provider checks the ID, URL, vault and secret name at plan time,
// so the fake doesn't repeat those checks for a new instance:
//   - a PUT over an instance that exists is refused if it changes or leaves out a
//     field, or if the instance is installed. An unchanged one at stage or configure is
//     staged again.
//   - a new instance needs an installed connector in its vault's cloud, and on AWS a
//     secret in another account than the connector's can't end like an ARN's suffix
//   - verify and configure refuse a change to any field, in the install framework's
//     words, and an instance whose connector is gone or isn't installed
//   - only verify runs the install check
func (f *argocdFake) refuseInstance(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
	f.instanceSteps = append(f.instanceSteps, step.nextState)
	id := step.key.id

	if step.nextState == "stage" {
		if step.previous != nil {
			if changedArgocdInstanceField(step.previous, step.body, true) != "" {
				return http.StatusConflict, fmt.Sprintf("An Argo CD instance with the ID %s already exists, and an instance can't be changed once it's added. "+
					"To add another instance, use another ID.", id)
			}
			if step.previous["state"] == "installed" {
				return http.StatusConflict, fmt.Sprintf("The Argo CD instance %s is already installed, and there's nothing to change.", id)
			}
			return 0, ""
		}
		connectorId, _ := step.body["connector"].(string)
		connector := items[fakeItemKey{"argocd", "connector", connectorId}]
		if connector == nil || connector["state"] != "installed" {
			return http.StatusBadRequest, "That Argo CD connector isn't installed in P0. Install it, and finish its setup, before you add an instance that uses it."
		}
		hosting, _ := connector["hosting"].(map[string]any)
		vault, _ := step.body["vault"].(map[string]any)
		vaultType, _ := vault["type"].(string)
		if argocdVaultCloud[vaultType] != hosting["type"] {
			return http.StatusBadRequest, map[any]string{
				"aws": "The instance's connector runs on AWS, so the API token's secret has to be in AWS Secrets Manager.",
				"gcp": "The instance's connector runs on Google Cloud, so the API token's secret has to be in Google Secret Manager.",
			}[hosting["type"]]
		}
		name, _ := step.body["secretName"].(string)
		if vaultType == "aws-sm" && vault["install"] != hosting["accountId"] && regexp.MustCompile(`-[A-Za-z0-9]{6}$`).MatchString(name) {
			return http.StatusBadRequest, "Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters, like -AbCdEf. " +
				"Use a secret whose name doesn't end that way, or one in the connector's own account."
		}
		return 0, ""
	}

	if field := changedArgocdInstanceField(step.previous, step.body, false); field != "" {
		return http.StatusUnprocessableEntity, fmt.Sprintf(
			"'%s' can only be altered on initial installation. Create a new installation to change this field.", field,
		)
	}
	connectorId, _ := step.previous["connector"].(string)
	connector := items[fakeItemKey{"argocd", "connector", connectorId}]
	switch {
	case connector == nil:
		return http.StatusUnprocessableEntity, fmt.Sprintf("The Argo CD connector %s, which the instance %s uses, isn't in P0 any more. "+
			"Add a connector with that ID, in the cloud that holds the instance's secret, then retry.", connectorId, id)
	case connector["state"] != "installed":
		return http.StatusUnprocessableEntity, fmt.Sprintf("The Argo CD connector %s, which the instance %s uses, isn't installed. Finish installing it, then retry.", connectorId, id)
	case step.nextState == "configure" && f.failedInstanceCheck != "":
		status := http.StatusUnprocessableEntity
		if f.instanceCheckUnreachable {
			status = http.StatusBadGateway
		}
		return status, f.failedInstanceCheck
	}
	return 0, ""
}

// The first field of a stored instance that body changes, by its path as P0 names it,
// or "" if body changes none.
// A PUT has to send the whole instance, so whole counts a field it leaves out as a
// change. Verify and configure are merged over the stored instance first.
func changedArgocdInstanceField(item, body map[string]any, whole bool) string {
	for _, field := range []string{"connector", "url", "managingApp", "namespace", "secretName"} {
		if sent, ok := body[field]; (ok || whole) && sent != item[field] {
			return field
		}
	}
	sent, ok := body["vault"].(map[string]any)
	if !ok {
		if whole {
			return "vault"
		}
		return ""
	}
	stored, _ := item["vault"].(map[string]any)
	if sent["type"] != stored["type"] {
		return "vault"
	}
	for _, field := range slices.Sorted(maps.Keys(sent)) {
		if sent[field] != stored[field] {
			return fmt.Sprintf("vault/%s/%s", sent["type"], field)
		}
	}
	return ""
}

func (f *argocdFake) connector() map[string]any {
	return f.item("argocd", "connector", argocdConnector)
}

// The connector's item in P0, if it is at state and holds exactly the hosting want.
func (f *argocdFake) checkConnector(state string, want map[string]any) error {
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

func checkArgocdDestroyed(f *argocdFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		return nil
	}
}

// Terraform wraps a diagnostic's text, so the words of each part of an error may be on
// different lines. The parts match in order, with anything between them.
func argocdError(parts ...string) *regexp.Regexp {
	patterns := []string{}
	for _, part := range parts {
		patterns = append(patterns, strings.Join(strings.Fields(regexp.QuoteMeta(part)), `\s+`))
	}
	return regexp.MustCompile(`(?s)` + strings.Join(patterns, `.*`))
}

// The connector on Google Cloud: staging it reads the service account that P0
// generates, and installing it reads the Cloud Run URL that P0 resolves.
func TestArgocdConnectorOnGoogleCloud(t *testing.T) {
	f := newArgocdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: argocdConnectorConfig(f, argocdGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_argocd_connector_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_argocd_connector_staged.example", "hosting.connector_service_account", argocdServiceAccount),
					resource.TestCheckNoResourceAttr("p0_argocd_connector_staged.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "hosting.project_id", argocdProject),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "hosting.connector_service_account", argocdServiceAccount),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "hosting.connector_service_uri", argocdServiceUri),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":                    "gcp",
							"projectId":               argocdProject,
							"connectorName":           argocdConnectorName,
							"connectorRegion":         "us-west1",
							"connectorServiceAccount": argocdServiceAccount,
							"connectorServiceUri":     argocdServiceUri,
						})
					},
				),
			},
			{
				ResourceName:      "p0_argocd_connector_staged.example",
				ImportState:       true,
				ImportStateId:     argocdConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state", "hosting.connector_service_uri"},
			},
			{
				ResourceName:      "p0_argocd_connector.example",
				ImportState:       true,
				ImportStateId:     argocdConnector,
				ImportStateVerify: true,
			},
			// The ID goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  "p0_argocd_connector.example",
				ImportState:   true,
				ImportStateId: "prod/other",
				ExpectError:   argocdError("Invalid Argo CD connector ID", "Use lowercase letters, digits and hyphens"),
			},
		},
	})
}

// The connector on AWS, where P0 generates nothing: it derives the function's address
// from the hosting.
func TestArgocdConnectorOnAws(t *testing.T) {
	f := newArgocdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: argocdConnectorConfig(f, argocdAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_argocd_connector_staged.example", "state", "stage"),
					resource.TestCheckNoResourceAttr("p0_argocd_connector_staged.example", "hosting.connector_service_account"),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_argocd_connector.example", "hosting.connector_service_account"),
					resource.TestCheckNoResourceAttr("p0_argocd_connector.example", "hosting.connector_service_uri"),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":            "aws",
							"accountId":       "123456789012",
							"connectorName":   argocdConnectorName,
							"connectorRegion": "us-west-2",
						})
					},
				),
			},
			{
				ResourceName:      "p0_argocd_connector.example",
				ImportState:       true,
				ImportStateId:     argocdConnector,
				ImportStateVerify: true,
			},
		},
	})
}

// A hosting that comes from a variable is unknown while Terraform validates the
// configuration, so the resources' checks skip it then, and known when it plans.
func TestArgocdConnectorHostingFromVariable(t *testing.T) {
	f := newArgocdFake(t)
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

resource "p0_argocd_connector_staged" "example" {
  id      = %q
  hosting = var.hosting
}

resource "p0_argocd_connector" "example" {
  id      = p0_argocd_connector_staged.example.id
  hosting = var.hosting
}
`, argocdGcp, argocdConnector)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_argocd_connector_staged.example", "hosting.connector_service_account", argocdServiceAccount),
					resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
				),
			},
		},
	})
}

// Where a connector runs is fixed when it is staged, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestArgocdConnectorHostingReplacesBoth(t *testing.T) {
	f := newArgocdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: argocdConnectorConfig(f, argocdGcp)},
			{
				Config: argocdConnectorConfig(f, argocdGcpHosting("connector_region", "europe-west1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_argocd_connector_staged.example", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					return f.checkConnector("installed", map[string]any{
						"type":                    "gcp",
						"projectId":               argocdProject,
						"connectorName":           argocdConnectorName,
						"connectorRegion":         "europe-west1",
						"connectorServiceAccount": argocdServiceAccount,
						"connectorServiceUri":     argocdServiceUri,
					})
				},
			},
		},
	})
}

// Destroying only the final resource returns its item to "stage", so the staged
// resource still owns it, and a later apply installs it again.
func TestArgocdConnectorRollback(t *testing.T) {
	f := newArgocdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: argocdConnectorConfig(f, argocdAws)},
			{
				Config: argocdStagedConfig(f, argocdAws),
				Check: func(*terraform.State) error {
					return f.checkConnector("stage", map[string]any{
						"type":            "aws",
						"accountId":       "123456789012",
						"connectorName":   argocdConnectorName,
						"connectorRegion": "us-west-2",
					})
				},
			},
			{
				Config: argocdConnectorConfig(f, argocdAws),
				Check:  resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
			},
		},
	})
}

// The final resource never stages the item, which would move the connector. It only
// verifies it, so P0 refuses a hosting that isn't the staged one and the connector
// stays where it was staged.
func TestArgocdConnectorMustMatchStaged(t *testing.T) {
	f := newArgocdFake(t)
	stagedAt := map[string]any{
		"type":                    "gcp",
		"projectId":               argocdProject,
		"connectorName":           argocdConnectorName,
		"connectorRegion":         "us-west1",
		"connectorServiceAccount": argocdServiceAccount,
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: argocdMovedConfig(f, argocdGcp, argocdGcpHosting("connector_region", "europe-west1")),
				ExpectError: argocdError(
					"Argo CD connector install check failed",
					"'hosting/gcp/connectorRegion' can only be altered on initial installation",
				),
			},
			{
				PreConfig: func() {
					if err := f.checkConnector("stage", stagedAt); err != nil {
						t.Errorf("after the refused install: %v", err)
					}
				},
				Config: argocdConnectorConfig(f, argocdGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_argocd_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
			},
		},
	})
}

// A connector that fails P0's install check fails the apply with P0's message, and
// stays staged. The next apply checks it again.
func TestArgocdConnectorFailedCheck(t *testing.T) {
	f := newArgocdFake(t)
	f.failedCheck = "P0 couldn't find the Cloud Run service p0-argocd-prod."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      argocdConnectorConfig(f, argocdGcp),
				ExpectError: argocdError("Argo CD connector install check failed", `Argo CD connector "prod"`, f.failedCheck),
			},
			{
				PreConfig: func() {
					if item := f.connector(); item == nil || item["state"] != "stage" {
						t.Errorf("P0 item after the failed check = %v; want it staged", item)
					}
					f.failedCheck = ""
				},
				Config: argocdConnectorConfig(f, argocdGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_argocd_connector_staged.example", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
			},
		},
	})
}

// On Google Cloud the connector can't be deployed without the service account P0
// generates, so a connector that comes back without one fails the apply and says so.
// On AWS there is none to miss.
func TestArgocdConnectorMissingServiceAccount(t *testing.T) {
	missing := argocdError("Bad API response", "Argo CD connector prod is hosted on Google Cloud, but P0 returned no service account for it")
	none := func(map[string]any) any { return nil }

	for name, account := range map[string]func(map[string]any) any{
		"no account":    none,
		"blank account": func(map[string]any) any { return "" },
	} {
		// The connector P0 staged isn't left behind, so once P0 returns the account the
		// next apply stages it.
		t.Run(name+" when staged on Google Cloud", func(t *testing.T) {
			f := newArgocdFake(t)
			working := f.serviceAccount
			f.serviceAccount = account
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkArgocdDestroyed(f),
				Steps: []resource.TestStep{
					{Config: argocdStagedConfig(f, argocdGcp), ExpectError: missing},
					{
						PreConfig: func() {
							if item := f.connector(); item != nil {
								t.Errorf("P0 item after the failed staging = %v; want none", item)
							}
							f.serviceAccount = working
						},
						Config: argocdStagedConfig(f, argocdGcp),
						Check:  resource.TestCheckResourceAttr("p0_argocd_connector_staged.example", "hosting.connector_service_account", argocdServiceAccount),
					},
				},
			})
		})
	}

	// The account goes missing after the connector is installed, as it would for an
	// item that someone changed in P0.
	t.Run("no account when read on Google Cloud", func(t *testing.T) {
		f := newArgocdFake(t)
		setAccount := func(account any) {
			f.update("argocd", "connector", argocdConnector, func(item map[string]any) {
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
			CheckDestroy:             checkArgocdDestroyed(f),
			Steps: []resource.TestStep{
				{Config: argocdConnectorConfig(f, argocdGcp)},
				{
					PreConfig: func() { setAccount(nil) },
					Config:    argocdConnectorConfig(f, argocdGcp),
					PlanOnly:  true,
					ExpectError: argocdError(
						"Bad API response", "Argo CD connector prod is hosted on Google Cloud, but P0 returned no service account for it",
						"plan and apply other changes with `-refresh=false`", "terraform state rm",
					),
				},
				// With the account back, the resources read again, and can be destroyed.
				{
					PreConfig: func() { setAccount(argocdServiceAccount) },
					Config:    argocdConnectorConfig(f, argocdGcp),
				},
			},
		})
	})

	t.Run("no account on AWS", func(t *testing.T) {
		f := newArgocdFake(t)
		f.serviceAccount = none
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdDestroyed(f),
			Steps: []resource.TestStep{
				{
					Config: argocdConnectorConfig(f, argocdAws),
					Check:  resource.TestCheckResourceAttr("p0_argocd_connector.example", "state", "installed"),
				},
			},
		})
	})
}

// A connector that P0 already holds, such as one added in the P0 app, isn't staged
// again: P0 would accept the PUT, which returns an installed connector to "stage" and
// stops every instance behind it. The apply fails, writes nothing, and says to import
// the connector. So does one whose read fails for another reason.
func TestArgocdConnectorStagedOverExisting(t *testing.T) {
	key := fakeItemKey{"argocd", "connector", argocdConnector}
	for _, state := range []string{"stage", "installed"} {
		t.Run(state, func(t *testing.T) {
			f := newArgocdFake(t)
			f.integrations["argocd"] = true
			f.items[key] = map[string]any{
				"hosting": map[string]any{
					"type": "gcp", "projectId": argocdProject, "connectorName": argocdConnectorName, "connectorRegion": "us-west1",
					"connectorServiceAccount": argocdServiceAccount,
				},
				"label": argocdConnector, "state": state,
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: argocdStagedConfig(f, argocdGcp),
						ExpectError: argocdError(
							"Argo CD connector already exists in P0", `P0 already has an Argo CD connector "prod"`,
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

// While an Argo CD instance uses the connector, P0 refuses to return it to "stage", so
// destroying p0_argocd_connector fails and says to remove the instances first. The
// connector stays installed, and goes once the instance has.
func TestArgocdConnectorInUse(t *testing.T) {
	f := newArgocdFake(t)
	instance := fakeItemKey{"argocd", "argocd-instance", "prod"}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: argocdConnectorConfig(f, argocdGcp)},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					f.items[instance] = map[string]any{"connector": argocdConnector, "state": "installed"}
				},
				Config: argocdStagedConfig(f, argocdGcp),
				ExpectError: argocdError(
					"Argo CD connector is in use", `the Argo CD connector "prod"`, "Remove each p0_argocd_instance that uses it first",
					"409 Conflict: The Argo CD connector prod can't be staged again while an Argo CD instance uses it (prod).",
				),
			},
			{
				PreConfig: func() {
					if item := f.connector(); item["state"] != "installed" {
						t.Errorf("P0 item = %v; want it still installed", item)
					}
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.items, instance)
				},
				Config: argocdStagedConfig(f, argocdGcp),
				Check: func(*terraform.State) error {
					if item := f.connector(); item["state"] != "stage" {
						return fmt.Errorf("P0 item = %v; want it staged", item)
					}
					return nil
				},
			},
		},
	})
}

// An answer from P0 with no item fails the refresh with a diagnostic, not a panic.
func TestArgocdConnectorAnswerWithoutItem(t *testing.T) {
	f := newArgocdFake(t)
	fake := f.server.Config.Handler
	path := "/o/" + fakeOrg + "/integrations/argocd/config/connector/" + argocdConnector

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: argocdStagedConfig(f, argocdGcp)},
			{
				PreConfig: func() {
					f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == http.MethodGet && r.URL.Path == path {
							writeJson(w, map[string]any{"ok": true})
							return
						}
						fake.ServeHTTP(w, r)
					})
				},
				Config:      argocdStagedConfig(f, argocdGcp),
				PlanOnly:    true,
				ExpectError: argocdError("Bad API response", "Could not read 'item'"),
			},
			{
				PreConfig: func() { f.server.Config.Handler = fake },
				Config:    argocdStagedConfig(f, argocdGcp),
			},
		},
	})
}

// A connector that P0 no longer has is dropped from state, as other resources are,
// and the next apply stages and installs it again.
func TestArgocdConnectorRemovedInP0(t *testing.T) {
	f := newArgocdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: argocdConnectorConfig(f, argocdGcp)},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.items, fakeItemKey{"argocd", "connector", argocdConnector})
				},
				Config: argocdConnectorConfig(f, argocdGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_argocd_connector_staged.example", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionCreate),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.connector(); item["state"] != "installed" {
						return fmt.Errorf("P0 item = %v; want the connector installed again", item)
					}
					return nil
				},
			},
		},
	})
}

// P0 has the last word on a value that the plan let through: its refusal fails the
// apply in its own words, and stages nothing.
func TestArgocdConnectorRefusedByP0(t *testing.T) {
	f := newArgocdFake(t)
	f.stageRefusal = "P0 couldn't use this connector name."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      argocdStagedConfig(f, argocdGcp),
				ExpectError: argocdError("Could not stage connector component", "400 Bad Request: P0 couldn't use this connector name."),
			},
		},
	})
}

func TestArgocdConnectorValidation(t *testing.T) {
	final := func(id, hosting string) string {
		return fmt.Sprintf(`
resource "p0_argocd_connector" "example" {
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
		{"id with capitals", argocdStaged("Prod", argocdGcp), []string{invalidId, "got: Prod"}},
		{"id ending with a hyphen", argocdStaged("prod-", argocdGcp), []string{invalidId}},
		{"id with a path", argocdStaged("prod/other", argocdGcp), []string{invalidId}},
		{"id over 63 characters", argocdStaged(strings.Repeat("a", 64), argocdGcp), []string{"string length must be at most 63"}},
		{"final id with capitals", final("Prod", argocdGcp), []string{invalidId}},
		{"AWS name ECR wouldn't take", argocdStaged("prod", argocdAwsHosting("connector_name", "p0--argocd")), []string{"Invalid Argo CD connector name", awsName, `Got: "p0--argocd"`}},
		{"AWS name over 64 characters", argocdStaged("prod", argocdAwsHosting("connector_name", "p"+strings.Repeat("0", 64))), []string{awsName}},
		{"AWS name with capitals", argocdStaged("prod", argocdAwsHosting("connector_name", "P0-Argo CD")), []string{awsName}},
		{"Google Cloud name too long for a service account", argocdStaged("prod", argocdGcpHosting("connector_name", "p0-argocd-connector-for-production")), []string{gcpName}},
		{"Google Cloud name too short for a service account", argocdStaged("prod", argocdGcpHosting("connector_name", "p0-ac")), []string{gcpName}},
		// P0 would trim the name and store it, and the apply would then fail on a name
		// that reads back different.
		{"name with whitespace around it", argocdStaged("prod", argocdGcpHosting("connector_name", argocdConnectorName+" ")), []string{gcpName, `Got: "p0-argocd-prod "`}},
		{"final name too long for a service account", final("prod", argocdGcpHosting("connector_name", "p0-argocd-connector-for-production")), []string{gcpName}},
		{
			"project on AWS hosting",
			argocdStaged("prod", `{
    type             = "aws"
    project_id       = "acme-prod"
    connector_name   = "p0-argocd-prod"
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
    connector_name   = "p0-argocd-prod"
    connector_region = "us-west1"
  }`),
			[]string{"Unexpected AWS account ID"},
		},
		{"project with an underscore", argocdStaged("prod", argocdGcpHosting("project_id", "acme_prod")), []string{"Google Cloud project identifiers must consist of alphanumeric characters and hyphens only"}},
		{"account that isn't one", argocdStaged("prod", argocdAwsHosting("account_id", "1234")), []string{"12 numeric digits"}},
		{"region with whitespace around it", argocdStaged("prod", argocdAwsHosting("connector_region", " us-west-2")), []string{"valid AWS or Google Cloud region"}},
		{
			"hosting on another cloud",
			argocdStaged("prod", `{
    type             = "azure"
    connector_name   = "p0-argocd-prod"
    connector_region = "westus2"
  }`),
			[]string{`value must be one of: ["aws" "gcp"]`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: argocdError(tc.errors...),
					},
				},
			})
		})
	}
}

// Terraform never plans an update of the staged resource, since each attribute that
// can be set requires replacement. One that reaches the provider anyway fails, rather
// than report a change it didn't make, or stage the connector again.
func TestArgocdConnectorStagedUpdateFails(t *testing.T) {
	var resp fwresource.UpdateResponse
	installargocd.NewArgocdConnectorStaged().Update(context.Background(), fwresource.UpdateRequest{}, &resp)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Please report this as a provider bug") {
		t.Errorf("diagnostics = %v; want one error asking for a bug report", resp.Diagnostics)
	}
}
