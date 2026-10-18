// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// A copy of databricks_test.go's case table, runners and account update tests, for Databricks (AI).

// What the Databricks (AI) tests install.
type databricksAiTestInstall struct {
	awsAccountId   string
	region         string
	accountId      string
	accountsUrl    string
	applicationId  string
	ownerPrincipal string
}

// What the tests against the fake install. They name nothing real.
var databricksAiTestDefaults = databricksAiTestInstall{
	awsAccountId:   "123456789012",
	region:         "us-west-2",
	accountId:      "01234567-89ab-cdef-0123-456789abcdef",
	accountsUrl:    "https://accounts.cloud.databricks.com",
	applicationId:  "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05",
	ownerPrincipal: "metastore-admins",
}

// The variables that name what TestAccDatabricksAi installs. P0 verifies every
// Databricks (AI) component, so they must name a deployed connector, and an
// account whose service principal it federates into. The principal must be
// account admin and metastore admin, and the owner principal must name one
// group, user or service principal in the account.
func databricksAiAccEnv(install *databricksAiTestInstall) map[string]*string {
	return map[string]*string{
		"P0_DATABRICKS_AI_AWS_ACCOUNT_ID":  &install.awsAccountId,
		"P0_DATABRICKS_AI_REGION":          &install.region,
		"P0_DATABRICKS_AI_ACCOUNT_ID":      &install.accountId,
		"P0_DATABRICKS_AI_ACCOUNTS_URL":    &install.accountsUrl,
		"P0_DATABRICKS_AI_APPLICATION_ID":  &install.applicationId,
		"P0_DATABRICKS_AI_OWNER_PRINCIPAL": &install.ownerPrincipal,
	}
}

// TestAccDatabricksAi installs into the P0 organization P0_ORG, served at
// P0_HOST (the provider's default host unless set), with the API token
// P0_API_TOKEN. Use an organization set aside for tests: they create and
// delete real installs.
const (
	databricksAiAccOrgEnv   = "P0_ORG"
	databricksAiAccHostEnv  = "P0_HOST"
	databricksAiAccTokenEnv = "P0_API_TOKEN"
	// The host that the provider calls unless its `host` says otherwise.
	databricksAiAccDefaultHost = "https://api.p0.app"
)

// testAccDatabricksAiPreCheck skips TestAccDatabricksAi unless it has a P0
// organization to run against and every P0_DATABRICKS_AI_* variable is set. It
// skips rather than fails, because CI sets TF_ACC without an organization. The
// fake's defaults name nothing real, so they can't install against P0.
func testAccDatabricksAiPreCheck(t *testing.T) {
	if os.Getenv(databricksAiAccOrgEnv) == "" || os.Getenv(databricksAiAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the Databricks (AI) acceptance tests against a P0 organization", databricksAiAccOrgEnv, databricksAiAccTokenEnv)
	}
	var missing []string
	for name := range databricksAiAccEnv(&databricksAiTestInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the Databricks (AI) acceptance tests", strings.Join(missing, ", "))
	}
}

func databricksAiAccHost() string {
	if host := os.Getenv(databricksAiAccHostEnv); host != "" {
		return host
	}
	return databricksAiAccDefaultHost
}

// databricksAiAccProviderConfig configures the provider for the
// acceptance-test organization. The provider reads the API token from
// P0_API_TOKEN itself.
func databricksAiAccProviderConfig() string {
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}
`, os.Getenv(databricksAiAccOrgEnv), databricksAiAccHost())
}

// databricksAiAccClient calls the acceptance-test organization's API directly,
// for checks that Terraform can't make, such as that a destroyed install is
// gone.
func databricksAiAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        databricksAiOrgUrl(databricksAiAccHost(), os.Getenv(databricksAiAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(databricksAiAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// databricksAiAccInstall reads what TestAccDatabricksAi installs from the
// P0_DATABRICKS_AI_* variables.
func databricksAiAccInstall() databricksAiTestInstall {
	var install databricksAiTestInstall
	for name, value := range databricksAiAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return install
}

// Each configuration installs one more component than the last, staging the
// connector and the account before installing them, as the examples do.

func (d databricksAiTestInstall) connectorStaged() string {
	return fmt.Sprintf(`
resource "p0_databricks_ai_connector_staged" "test" {
  id     = %q
  region = %q
}
`, d.awsAccountId, d.region)
}

func (d databricksAiTestInstall) connector() string {
	return d.connectorStaged() + `
resource "p0_databricks_ai_connector" "test" {
  id     = p0_databricks_ai_connector_staged.test.id
  region = p0_databricks_ai_connector_staged.test.region
}
`
}

func (d databricksAiTestInstall) accountStaged() string {
	return d.connector() + fmt.Sprintf(`
resource "p0_databricks_ai_account_staged" "test" {
  id           = %q
  connector    = p0_databricks_ai_connector.test.id
  accounts_url = %q
}
`, d.accountId, d.accountsUrl)
}

func (d databricksAiTestInstall) account() string {
	return d.accountStaged() + fmt.Sprintf(`
resource "p0_databricks_ai_account" "test" {
  id              = p0_databricks_ai_account_staged.test.id
  connector       = p0_databricks_ai_account_staged.test.connector
  accounts_url    = p0_databricks_ai_account_staged.test.accounts_url
  application_id  = %q
  owner_principal = %q
}
`, d.applicationId, d.ownerPrincipal)
}

// One resource's test: install everything up to it, check its attributes,
// import it, and check that destroying the configuration deletes every item.
type databricksAiCase struct {
	name     string
	resource string
	config   func(databricksAiTestInstall) string
	want     func(databricksAiTestInstall) map[string]string
}

var databricksAiCases = []databricksAiCase{
	{
		name:     "ConnectorStaged",
		resource: "p0_databricks_ai_connector_staged.test",
		config:   databricksAiTestInstall.connectorStaged,
		want: func(d databricksAiTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "federation_audience": "databricks", "state": "stage"}
		},
	},
	{
		name:     "Connector",
		resource: "p0_databricks_ai_connector.test",
		config:   databricksAiTestInstall.connector,
		want: func(d databricksAiTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "federation_audience": "databricks", "state": "installed"}
		},
	},
	{
		name:     "AccountStaged",
		resource: "p0_databricks_ai_account_staged.test",
		config:   databricksAiTestInstall.accountStaged,
		want: func(d databricksAiTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "federation_audience": "databricks", "state": "stage"}
		},
	},
	{
		name:     "Account",
		resource: "p0_databricks_ai_account.test",
		config:   databricksAiTestInstall.account,
		want: func(d databricksAiTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "application_id": d.applicationId, "owner_principal": d.ownerPrincipal, "federation_audience": "databricks", "state": "installed"}
		},
	},
}

func (c databricksAiCase) testCase(provider string, client *internal.P0ProviderData, install databricksAiTestInstall, preCheck func()) resource.TestCase {
	var checks []resource.TestCheckFunc
	for attribute, value := range c.want(install) {
		checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
	}
	config := provider + c.config(install)

	return resource.TestCase{
		PreCheck:                 preCheck,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksAiDestroyed(client),
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

var databricksAiComponentByResourceType = map[string]string{
	"p0_databricks_ai_connector_staged": "connector",
	"p0_databricks_ai_connector":        "connector",
	"p0_databricks_ai_account_staged":   "account",
	"p0_databricks_ai_account":          "account",
}

func databricksAiItemPath(component, id string) string {
	return fmt.Sprintf("integrations/databricks-ai/config/%s/%s", component, id)
}

// checkDatabricksAiDestroyed checks that P0 no longer has the item behind any
// Databricks (AI) resource in the destroyed state.
func checkDatabricksAiDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := databricksAiComponentByResourceType[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(databricksAiItemPath(component, rs.Primary.ID), &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the Databricks (AI) %s %s was deleted: %w", component, rs.Primary.ID, err)
			}
			return fmt.Errorf("the Databricks (AI) %s %s is still installed in P0", component, rs.Primary.ID)
		}
		return nil
	}
}

// TestAccDatabricksAi installs each Databricks (AI) resource in a P0
// organization (see testAccDatabricksAiPreCheck). The organization must run a
// P0 version with the Databricks (AI) integration, and have it enabled.
func TestAccDatabricksAi(t *testing.T) {
	for _, c := range databricksAiCases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, c.testCase(databricksAiAccProviderConfig(), databricksAiAccClient(), databricksAiAccInstall(), func() { testAccDatabricksAiPreCheck(t) }))
		})
	}
}

// TestDatabricksAi installs each Databricks (AI) resource against the fake, so
// it needs neither TF_ACC nor a P0 organization.
func TestDatabricksAi(t *testing.T) {
	for _, c := range databricksAiCases {
		t.Run(c.name, func(t *testing.T) {
			f := newDatabricksAiFake(t)
			resource.UnitTest(t, c.testCase(f.providerConfig(), f.client(), databricksAiTestDefaults, nil))
		})
	}
}

// A recreated service principal's application ID, for the update tests.
const databricksAiNewApplicationId = "0d26daa6-5e44-4c97-a497-ef015f91254a"

// P0 accepts a new application ID when it configures an account, so a
// recreated service principal updates the account in place.
func TestDatabricksAiAccountUpdatedOnNewApplicationId(t *testing.T) {
	f := newDatabricksAiFake(t)
	changed := databricksAiTestDefaults
	changed.applicationId = databricksAiNewApplicationId

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + databricksAiTestDefaults.account()},
			{
				Config: f.providerConfig() + changed.account(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_ai_account.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_ai_account.test", "application_id", changed.applicationId),
					resource.TestCheckResourceAttr("p0_databricks_ai_account.test", "state", "installed"),
				),
			},
		},
	})
}

// P0 checks a new application ID on the configure step alone, which saves
// nothing when the check fails. So a failed update leaves the account installed
// with its current ID, which grants and revokes in it keep using, and the next
// plan shows the update again.
func TestDatabricksAiAccountKeptWhenNewApplicationIdFails(t *testing.T) {
	f := newDatabricksAiFake(t)
	d := databricksAiTestDefaults
	changed := d
	changed.applicationId = databricksAiNewApplicationId
	// The new service principal can't sign in yet, for example because its
	// federation policy isn't applied, so P0's check on the configure step fails.
	var refuseConfigure atomic.Bool
	f.refuse = func(key fakeItemKey, nextState string) bool {
		return refuseConfigure.Load() && key.component == "account" && nextState == "installed"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.account()},
			{
				PreConfig:   func() { refuseConfigure.Store(true) },
				Config:      f.providerConfig() + changed.account(),
				ExpectError: regexp.MustCompile(`Databricks \(AI\) install check failed`),
			},
			{
				PreConfig: func() {
					refuseConfigure.Store(false)
					if item := f.item("databricks-ai", "account", d.accountId); item["state"] != "installed" || item["applicationId"] != d.applicationId {
						t.Errorf("after the failed update, P0 has the account %v, want it installed with application ID %s", item, d.applicationId)
					}
				},
				Config: f.providerConfig() + changed.account(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_ai_account.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_ai_account.test", "application_id", changed.applicationId),
					resource.TestCheckResourceAttr("p0_databricks_ai_account.test", "state", "installed"),
				),
			},
		},
	})
}
