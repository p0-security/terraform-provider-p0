// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// A copy of redshift_ai_test.go, trimmed to the connector, for AWS SSM (AI).

// What the AWS SSM (AI) tests install.
type awsSsmAiTestInstall struct {
	awsAccountId    string
	region          string
	vaultAccountId  string
	secretsRegion   string
	instanceRegions []string
	// Each is left out of the configuration when nil.
	runAsUsers    []string
	isSudoEnabled *bool
}

// What the tests against the fake install. They name nothing real.
var awsSsmAiTestDefaults = awsSsmAiTestInstall{
	awsAccountId:    "123456789012",
	region:          "us-west-2",
	vaultAccountId:  "123456789012",
	secretsRegion:   "us-west-2",
	instanceRegions: []string{"us-west-2", "us-east-1"},
}

// The variables that name what TestAccAwsSsmAi installs. P0 verifies the
// connector, so they must name a deployed connector in an account with P0's
// AWS IAM management, AWS OIDC and AWS Secrets Manager integrations. The
// instance regions are separated by commas.
func awsSsmAiAccEnv(install *awsSsmAiAccValues) map[string]*string {
	return map[string]*string{
		"P0_AWS_SSM_AI_AWS_ACCOUNT_ID":   &install.awsAccountId,
		"P0_AWS_SSM_AI_REGION":           &install.region,
		"P0_AWS_SSM_AI_SECRETS_REGION":   &install.secretsRegion,
		"P0_AWS_SSM_AI_INSTANCE_REGIONS": &install.instanceRegions,
	}
}

// The P0_AWS_SSM_AI_* variables, as read.
type awsSsmAiAccValues struct {
	awsAccountId, region, secretsRegion, instanceRegions string
}

// TestAccAwsSsmAi installs into the P0 organization P0_ORG, served at P0_HOST
// (the provider's default host unless set), with the API token P0_API_TOKEN.
// Use an organization set aside for tests: they create and delete real
// installs.
const (
	awsSsmAiAccOrgEnv   = "P0_ORG"
	awsSsmAiAccHostEnv  = "P0_HOST"
	awsSsmAiAccTokenEnv = "P0_API_TOKEN"
	// The host that the provider calls unless its `host` says otherwise.
	awsSsmAiAccDefaultHost = "https://api.p0.app"
)

// testAccAwsSsmAiPreCheck skips TestAccAwsSsmAi unless it has a P0 organization to
// run against and every P0_AWS_SSM_AI_* variable is set. It skips rather than
// fails, because CI sets TF_ACC without an organization. The fake's defaults
// name nothing real, so they can't install against P0.
func testAccAwsSsmAiPreCheck(t *testing.T) {
	if os.Getenv(awsSsmAiAccOrgEnv) == "" || os.Getenv(awsSsmAiAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the AWS SSM (AI) acceptance tests against a P0 organization", awsSsmAiAccOrgEnv, awsSsmAiAccTokenEnv)
	}
	var missing []string
	for name := range awsSsmAiAccEnv(&awsSsmAiAccValues{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the AWS SSM (AI) acceptance tests", strings.Join(missing, ", "))
	}
}

func awsSsmAiAccHost() string {
	if host := os.Getenv(awsSsmAiAccHostEnv); host != "" {
		return host
	}
	return awsSsmAiAccDefaultHost
}

// awsSsmAiAccProviderConfig configures the provider for the acceptance-test
// organization. The provider reads the API token from P0_API_TOKEN itself.
func awsSsmAiAccProviderConfig() string {
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}
`, os.Getenv(awsSsmAiAccOrgEnv), awsSsmAiAccHost())
}

// awsSsmAiAccClient calls the acceptance-test organization's API directly, for
// checks that Terraform can't make, such as that a destroyed install is gone.
func awsSsmAiAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        awsSsmAiOrgUrl(awsSsmAiAccHost(), os.Getenv(awsSsmAiAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(awsSsmAiAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// awsSsmAiAccInstall reads what TestAccAwsSsmAi installs from the P0_AWS_SSM_AI_*
// variables. The vault is in the connector's own account, as P0 requires.
func awsSsmAiAccInstall() awsSsmAiTestInstall {
	var values awsSsmAiAccValues
	for name, value := range awsSsmAiAccEnv(&values) {
		*value = os.Getenv(name)
	}
	var instanceRegions []string
	for _, region := range strings.Split(values.instanceRegions, ",") {
		instanceRegions = append(instanceRegions, strings.TrimSpace(region))
	}
	return awsSsmAiTestInstall{
		awsAccountId:    values.awsAccountId,
		region:          values.region,
		vaultAccountId:  values.awsAccountId,
		secretsRegion:   values.secretsRegion,
		instanceRegions: instanceRegions,
	}
}

// The configuration stages the connector before installing it, as the
// examples do.

func (d awsSsmAiTestInstall) connectorStaged() string {
	return fmt.Sprintf(`
resource "p0_aws_ssm_ai_connector_staged" "test" {
  id     = %q
  region = %q
  vault = {
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }
}
`, d.awsAccountId, d.region, d.vaultAccountId, d.secretsRegion)
}

// Leaves run_as_users and is_sudo_enabled out when the install doesn't set
// them.
func (d awsSsmAiTestInstall) connector() string {
	optional := ""
	if d.runAsUsers != nil {
		optional += "\n  run_as_users     = " + awsSsmAiHclList(d.runAsUsers)
	}
	if d.isSudoEnabled != nil {
		optional += "\n  is_sudo_enabled  = " + strconv.FormatBool(*d.isSudoEnabled)
	}
	return d.connectorStaged() + fmt.Sprintf(`
resource "p0_aws_ssm_ai_connector" "test" {
  id               = p0_aws_ssm_ai_connector_staged.test.id
  region           = p0_aws_ssm_ai_connector_staged.test.region
  vault            = p0_aws_ssm_ai_connector_staged.test.vault
  instance_regions = %s%s
}
`, awsSsmAiHclList(d.instanceRegions), optional)
}

// A list of strings in HCL.
func awsSsmAiHclList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// One resource's test: install everything up to it, check its attributes,
// import it, and check that destroying the configuration deletes the item.
type awsSsmAiCase struct {
	name     string
	resource string
	config   func(awsSsmAiTestInstall) string
	want     func(awsSsmAiTestInstall) map[string]string
}

// The attributes that both resources have.
func awsSsmAiStagedWant(d awsSsmAiTestInstall, state string) map[string]string {
	return map[string]string{
		"id":                   d.awsAccountId,
		"region":               d.region,
		"vault.type":           "aws-sm",
		"vault.account_id":     d.vaultAccountId,
		"vault.secrets_region": d.secretsRegion,
		"state":                state,
	}
}

var awsSsmAiCases = []awsSsmAiCase{
	{
		name:     "ConnectorStaged",
		resource: "p0_aws_ssm_ai_connector_staged.test",
		config:   awsSsmAiTestInstall.connectorStaged,
		want: func(d awsSsmAiTestInstall) map[string]string {
			return awsSsmAiStagedWant(d, "stage")
		},
	},
	{
		name:     "Connector",
		resource: "p0_aws_ssm_ai_connector.test",
		config:   awsSsmAiTestInstall.connector,
		want: func(d awsSsmAiTestInstall) map[string]string {
			want := awsSsmAiStagedWant(d, "installed")
			want["instance_regions.#"] = strconv.Itoa(len(d.instanceRegions))
			for i, region := range d.instanceRegions {
				want[fmt.Sprintf("instance_regions.%d", i)] = region
			}
			want["is_sudo_enabled"] = "false"
			return want
		},
	},
}

func (c awsSsmAiCase) testCase(provider string, client *internal.P0ProviderData, install awsSsmAiTestInstall, preCheck func()) resource.TestCase {
	var checks []resource.TestCheckFunc
	for attribute, value := range c.want(install) {
		checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
	}
	config := provider + c.config(install)

	return resource.TestCase{
		PreCheck:                 preCheck,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAwsSsmAiDestroyed(client),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
			{
				Config:            config,
				ResourceName:      c.resource,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	}
}

var awsSsmAiResourceTypes = []string{"p0_aws_ssm_ai_connector_staged", "p0_aws_ssm_ai_connector"}

// checkAwsSsmAiDestroyed checks that P0 no longer has the item behind any SSH
// (AI) resource in the destroyed state.
func checkAwsSsmAiDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			if !slices.Contains(awsSsmAiResourceTypes, rs.Type) {
				continue
			}
			resp, err := client.Get("integrations/aws-ssm-ai/config/connector/"+rs.Primary.ID, &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the AWS SSM (AI) connector %s was deleted: %w", rs.Primary.ID, err)
			}
			return fmt.Errorf("the AWS SSM (AI) connector %s is still installed in P0", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccAwsSsmAi installs each AWS SSM (AI) resource in a P0 organization (see
// testAccAwsSsmAiPreCheck). The organization must run a P0 version with the SSH
// (AI) integration.
func TestAccAwsSsmAi(t *testing.T) {
	for _, c := range awsSsmAiCases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, c.testCase(awsSsmAiAccProviderConfig(), awsSsmAiAccClient(), awsSsmAiAccInstall(), func() { testAccAwsSsmAiPreCheck(t) }))
		})
	}
}

// TestAwsSsmAi installs each AWS SSM (AI) resource against the fake, so it needs
// neither TF_ACC nor a P0 organization.
func TestAwsSsmAi(t *testing.T) {
	for _, c := range awsSsmAiCases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			resource.UnitTest(t, c.testCase(f.providerConfig(), f.client(), awsSsmAiTestDefaults, nil))
		})
	}
}

// P0 takes the instance regions and the run-as users each as one string,
// separated by commas, and the provider reads them back as lists. A
// configuration without run-as users sends an empty string.
func TestAwsSsmAiConnectorSendsLists(t *testing.T) {
	cases := []struct {
		name           string
		runAsUsers     []string
		wantRunAsUsers string
	}{
		{name: "NoRunAsUsers"},
		{name: "RunAsUsers", runAsUsers: []string{"root", "deploy"}, wantRunAsUsers: "root,deploy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			d := awsSsmAiTestDefaults
			d.runAsUsers = c.runAsUsers

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{
						Config: f.providerConfig() + d.connector(),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "run_as_users.#", strconv.Itoa(len(c.runAsUsers))),
							func(*terraform.State) error {
								item := f.item("aws-ssm-ai", "connector", d.awsAccountId)
								if item["instanceRegions"] != "us-west-2,us-east-1" || item["runAsUsers"] != c.wantRunAsUsers {
									return fmt.Errorf("P0 has the connector %v, want instanceRegions \"us-west-2,us-east-1\" and runAsUsers %q", item, c.wantRunAsUsers)
								}
								return nil
							},
						),
					},
				},
			})
		})
	}
}

func awsSsmAiBool(value bool) *bool {
	return &value
}

// P0 accepts new instance regions or run-as users when it configures the
// connector, so either updates it in place. Removing run_as_users clears them
// in P0. The deprecated is_sudo_enabled sends root as the only run-as user,
// and the plan after each apply shows no change.
func TestAwsSsmAiConnectorUpdatedInPlace(t *testing.T) {
	cases := []struct {
		name                string
		from                func(*awsSsmAiTestInstall)
		to                  func(*awsSsmAiTestInstall)
		wantInstanceRegions string
		wantRunAsUsers      string
		wantIsSudoEnabled   bool
	}{
		{
			name:                "InstanceRegions",
			from:                func(*awsSsmAiTestInstall) {},
			to:                  func(d *awsSsmAiTestInstall) { d.instanceRegions = []string{"eu-west-1"} },
			wantInstanceRegions: "eu-west-1",
		},
		{
			name:                "RunAsUsersAdded",
			from:                func(*awsSsmAiTestInstall) {},
			to:                  func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"deploy", "root"} },
			wantInstanceRegions: "us-west-2,us-east-1",
			wantRunAsUsers:      "deploy,root",
		},
		{
			name:                "RunAsUsersChanged",
			from:                func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"root"} },
			to:                  func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"deploy"} },
			wantInstanceRegions: "us-west-2,us-east-1",
			wantRunAsUsers:      "deploy",
		},
		{
			name:                "RunAsUsersRemoved",
			from:                func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"root"} },
			to:                  func(d *awsSsmAiTestInstall) { d.runAsUsers = nil },
			wantInstanceRegions: "us-west-2,us-east-1",
		},
		{
			name:                "SudoEnabled",
			from:                func(*awsSsmAiTestInstall) {},
			to:                  func(d *awsSsmAiTestInstall) { d.isSudoEnabled = awsSsmAiBool(true) },
			wantInstanceRegions: "us-west-2,us-east-1",
			wantRunAsUsers:      "root",
			wantIsSudoEnabled:   true,
		},
		{
			name:                "SudoRemoved",
			from:                func(d *awsSsmAiTestInstall) { d.isSudoEnabled = awsSsmAiBool(true) },
			to:                  func(d *awsSsmAiTestInstall) { d.isSudoEnabled = nil },
			wantInstanceRegions: "us-west-2,us-east-1",
		},
		{
			name: "SudoToRunAsUsers",
			from: func(d *awsSsmAiTestInstall) { d.isSudoEnabled = awsSsmAiBool(true) },
			to: func(d *awsSsmAiTestInstall) {
				d.isSudoEnabled = nil
				d.runAsUsers = []string{"root"}
			},
			wantInstanceRegions: "us-west-2,us-east-1",
			wantRunAsUsers:      "root",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			from, to := awsSsmAiTestDefaults, awsSsmAiTestDefaults
			c.from(&from)
			c.to(&to)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{Config: f.providerConfig() + from.connector()},
					{
						Config: f.providerConfig() + to.connector(),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", plancheck.ResourceActionUpdate),
								plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector_staged.test", plancheck.ResourceActionNoop),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "is_sudo_enabled", strconv.FormatBool(c.wantIsSudoEnabled)),
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "state", "installed"),
							func(*terraform.State) error {
								item := f.item("aws-ssm-ai", "connector", from.awsAccountId)
								if item["instanceRegions"] != c.wantInstanceRegions || item["runAsUsers"] != c.wantRunAsUsers {
									return fmt.Errorf("P0 has the connector %v, want instanceRegions %q and runAsUsers %q", item, c.wantInstanceRegions, c.wantRunAsUsers)
								}
								return nil
							},
						),
					},
				},
			})
		})
	}
}

// The state keeps the deprecated switch only while P0 has root as the only
// run-as user. Any other list in P0 shows in the next plan, which puts root
// back.
func TestAwsSsmAiConnectorSudoDrift(t *testing.T) {
	f := newAwsSsmAiFake(t)
	d := awsSsmAiTestDefaults
	d.isSudoEnabled = awsSsmAiBool(true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.connector()},
			{
				PreConfig: func() {
					f.update("aws-ssm-ai", "connector", d.awsAccountId, func(item map[string]any) {
						item["runAsUsers"] = "root,deploy"
					})
				},
				Config: f.providerConfig() + d.connector(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "is_sudo_enabled", "true"),
					resource.TestCheckNoResourceAttr("p0_aws_ssm_ai_connector.test", "run_as_users"),
					func(*terraform.State) error {
						if item := f.item("aws-ssm-ai", "connector", d.awsAccountId); item["runAsUsers"] != "root" {
							return fmt.Errorf("P0 has the connector %v, want runAsUsers \"root\"", item)
						}
						return nil
					},
				),
			},
		},
	})
}

// P0 lets agents run as root on a connector installed before run-as users
// while its old sudo switch is on, so the provider reads it as root. A
// configuration with the switch shows no change, and one without it clears
// root in P0.
func TestAwsSsmAiConnectorSudoInstalledBefore(t *testing.T) {
	cases := []struct {
		name           string
		isSudoEnabled  *bool
		wantAction     plancheck.ResourceActionType
		wantRunAsUsers any
	}{
		{name: "SudoKept", isSudoEnabled: awsSsmAiBool(true), wantAction: plancheck.ResourceActionNoop, wantRunAsUsers: nil},
		{name: "SudoRemoved", wantAction: plancheck.ResourceActionUpdate, wantRunAsUsers: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			before := awsSsmAiTestDefaults
			before.isSudoEnabled = awsSsmAiBool(true)
			after := awsSsmAiTestDefaults
			after.isSudoEnabled = c.isSudoEnabled

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{Config: f.providerConfig() + before.connector()},
					{
						// As the previous provider left it.
						PreConfig: func() {
							f.update("aws-ssm-ai", "connector", before.awsAccountId, func(item map[string]any) {
								delete(item, "runAsUsers")
								item["isSudoEnabled"] = true
							})
						},
						Config: f.providerConfig() + after.connector(),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", c.wantAction),
							},
						},
						Check: func(*terraform.State) error {
							if item := f.item("aws-ssm-ai", "connector", before.awsAccountId); item["runAsUsers"] != c.wantRunAsUsers {
								return fmt.Errorf("P0 has the connector %v, want runAsUsers %v", item, c.wantRunAsUsers)
							}
							return nil
						},
					},
				},
			})
		})
	}
}

// P0 refuses to change the region or the vault after staging, so a new one
// replaces both resources.
func TestAwsSsmAiConnectorReplaced(t *testing.T) {
	cases := []struct {
		name string
		to   func(*awsSsmAiTestInstall)
	}{
		{
			name: "Region",
			to:   func(d *awsSsmAiTestInstall) { d.region = "eu-west-1" },
		},
		{
			name: "SecretsRegion",
			to:   func(d *awsSsmAiTestInstall) { d.secretsRegion = "eu-west-1" },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			from, to := awsSsmAiTestDefaults, awsSsmAiTestDefaults
			c.to(&to)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{Config: f.providerConfig() + from.connector()},
					{
						Config: f.providerConfig() + to.connector(),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector_staged.test", plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", plancheck.ResourceActionReplace),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "region", to.region),
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "vault.secrets_region", to.secretsRegion),
							resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "state", "installed"),
						),
					},
				},
			})
		})
	}
}

// P0 checks new instance regions on the configure step alone, which saves
// nothing when the check fails. So a failed update leaves the connector
// installed with its current regions, and the next plan shows the update
// again.
func TestAwsSsmAiConnectorKeptWhenUpdateFails(t *testing.T) {
	f := newAwsSsmAiFake(t)
	d := awsSsmAiTestDefaults
	changed := d
	// The connector can't reach SSM in the new region.
	changed.instanceRegions = []string{"us-west-2", "eu-west-1"}
	var refuseConfigure atomic.Bool
	f.failCheck = func(_ fakeItemKey, nextState string) bool {
		return refuseConfigure.Load() && nextState == "installed"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.connector()},
			{
				PreConfig:   func() { refuseConfigure.Store(true) },
				Config:      f.providerConfig() + changed.connector(),
				ExpectError: regexp.MustCompile(`AWS SSM \(AI\) install check failed`),
			},
			{
				PreConfig: func() {
					refuseConfigure.Store(false)
					if item := f.item("aws-ssm-ai", "connector", d.awsAccountId); item["state"] != "installed" || item["instanceRegions"] != "us-west-2,us-east-1" {
						t.Errorf("after the failed update, P0 has the connector %v, want it installed with instanceRegions us-west-2,us-east-1", item)
					}
				},
				Config: f.providerConfig() + changed.connector(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "instance_regions.1", "eu-west-1"),
					resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "state", "installed"),
				),
			},
		},
	})
}

// A connector that P0 hasn't installed, such as one imported while staged, has
// no instance regions in P0. The apply that finishes the install sends them,
// and verifies and configures the connector.
func TestAwsSsmAiConnectorFinishedFromStage(t *testing.T) {
	f := newAwsSsmAiFake(t)
	d := awsSsmAiTestDefaults

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkAwsSsmAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.connectorStaged()},
			{
				Config:             f.providerConfig() + d.connector(),
				ResourceName:       "p0_aws_ssm_ai_connector.test",
				ImportState:        true,
				ImportStateId:      d.awsAccountId,
				ImportStatePersist: true,
			},
			{
				Config: f.providerConfig() + d.connector(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_aws_ssm_ai_connector.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "instance_regions.#", "2"),
					resource.TestCheckResourceAttr("p0_aws_ssm_ai_connector.test", "state", "installed"),
				),
			},
		},
	})
}

// The provider refuses at plan time what P0 would refuse: a vault outside the
// connector's account, a region outside AWS's commercial partition, an empty
// or repeated list of instance regions or run-as users, and a run-as user that
// isn't a Linux user name. It also refuses run_as_users with the deprecated
// is_sudo_enabled. Terraform wraps long messages, so a space in want matches
// any whitespace.
func TestAwsSsmAiConnectorInvalid(t *testing.T) {
	cases := []struct {
		name   string
		staged bool
		change func(*awsSsmAiTestInstall)
		want   string
	}{
		{name: "VaultAccount", change: func(d *awsSsmAiTestInstall) { d.vaultAccountId = "210987654321" }, want: `Choose the connector's own AWS account`},
		{name: "StagedVaultAccount", staged: true, change: func(d *awsSsmAiTestInstall) { d.vaultAccountId = "210987654321" }, want: `Choose the connector's own AWS account`},
		{name: "AccountId", staged: true, change: func(d *awsSsmAiTestInstall) { d.awsAccountId = "1234" }, want: `AWS account IDs should consist of 12 numeric digits`},
		{name: "Region", staged: true, change: func(d *awsSsmAiTestInstall) { d.region = "us-gov-west-1" }, want: `Choose a commercial AWS region`},
		{name: "RegionWhitespace", staged: true, change: func(d *awsSsmAiTestInstall) { d.region = " us-west-2" }, want: `Invalid AWS region`},
		{name: "SecretsRegion", staged: true, change: func(d *awsSsmAiTestInstall) { d.secretsRegion = "cn-north-1" }, want: `Enter a commercial AWS region for the secrets`},
		{name: "InstanceRegion", change: func(d *awsSsmAiTestInstall) { d.instanceRegions = []string{"us-west-2", "us-gov-east-1"} }, want: `Enter commercial AWS regions`},
		{name: "DuplicateInstanceRegions", change: func(d *awsSsmAiTestInstall) { d.instanceRegions = []string{"us-west-2", "us-west-2"} }, want: `Duplicate List Value`},
		{name: "NoInstanceRegions", change: func(d *awsSsmAiTestInstall) { d.instanceRegions = nil }, want: `must contain at least 1`},
		{name: "RunAsUser", change: func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"root", "Deploy"} }, want: `must be a Linux user name`},
		{name: "RunAsUserWithComma", change: func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"root,deploy"} }, want: `must be a Linux user name`},
		{name: "DuplicateRunAsUsers", change: func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{"root", "root"} }, want: `Duplicate List Value`},
		{name: "NoRunAsUsers", change: func(d *awsSsmAiTestInstall) { d.runAsUsers = []string{} }, want: `must contain at least 1`},
		{
			name: "RunAsUsersWithSudo",
			change: func(d *awsSsmAiTestInstall) {
				d.runAsUsers = []string{"root"}
				d.isSudoEnabled = awsSsmAiBool(true)
			},
			want: `Invalid Attribute Combination`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAwsSsmAiFake(t)
			d := awsSsmAiTestDefaults
			c.change(&d)
			config := d.connector()
			if c.staged {
				config = d.connectorStaged()
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      f.providerConfig() + config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(strings.ReplaceAll(c.want, " ", `\s+`)),
					},
				},
			})
		})
	}
}

// A null run-as user would reach P0 as an empty name, so the provider refuses
// it at plan time.
func TestAwsSsmAiConnectorNullRunAsUser(t *testing.T) {
	f := newAwsSsmAiFake(t)
	d := awsSsmAiTestDefaults
	d.runAsUsers = []string{"root"}
	config := strings.Replace(d.connector(), `["root"]`, `["root", null]`, 1)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.providerConfig() + config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Null List Value`),
			},
		},
	})
}

// The vault takes only AWS Secrets Manager, since the connector and the
// agents' MCP servers read no other.
func TestAwsSsmAiConnectorVaultType(t *testing.T) {
	f := newAwsSsmAiFake(t)
	config := strings.Replace(awsSsmAiTestDefaults.connectorStaged(), `"aws-sm"`, `"gcp-sm"`, 1)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.providerConfig() + config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}
