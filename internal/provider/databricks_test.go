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
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// The Databricks install spec, as the app defines it in
// packages/integrations/databricks/src/shared/components.ts.
var databricksStubSpec = stubInstallSpec{
	integration: "databricks",
	components: map[string]stubComponent{
		"connector": {fields: []string{"region", "domainPattern"}, stepNew: []string{"region", "domainPattern"}},
		"account":   {fields: []string{"connector", "accountsUrl", "applicationId"}, stepNew: []string{"connector", "accountsUrl"}},
		"workspace": {fields: []string{"account"}, stepNew: []string{"account"}},
		"catalog":   {fields: []string{"workspace"}, stepNew: []string{"workspace"}},
	},
}

// What the Databricks tests install.
type databricksTestInstall struct {
	awsAccountId  string
	region        string
	domainPattern string
	accountId     string
	accountsUrl   string
	applicationId string
	workspaceId   string
	catalog       string
}

// What the tests against the install API stub install. They name nothing real.
var databricksTestDefaults = databricksTestInstall{
	awsAccountId:  "123456789012",
	region:        "us-west-2",
	domainPattern: `example\.com`,
	accountId:     "01234567-89ab-cdef-0123-456789abcdef",
	accountsUrl:   "https://accounts.cloud.databricks.com",
	applicationId: "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05",
	workspaceId:   "1234567890123456",
	catalog:       "p0_acceptance_test",
}

// The variables that name what TestAccDatabricks installs. P0 verifies every
// Databricks component, so they must name a deployed connector, an account
// whose service principal it federates into, and a workspace and catalog that
// the principal administers.
func databricksAccEnv(install *databricksTestInstall) map[string]*string {
	return map[string]*string{
		"P0_DATABRICKS_AWS_ACCOUNT_ID": &install.awsAccountId,
		"P0_DATABRICKS_REGION":         &install.region,
		"P0_DATABRICKS_DOMAIN_PATTERN": &install.domainPattern,
		"P0_DATABRICKS_ACCOUNT_ID":     &install.accountId,
		"P0_DATABRICKS_ACCOUNTS_URL":   &install.accountsUrl,
		"P0_DATABRICKS_APPLICATION_ID": &install.applicationId,
		"P0_DATABRICKS_WORKSPACE_ID":   &install.workspaceId,
		"P0_DATABRICKS_CATALOG":        &install.catalog,
	}
}

// testAccDatabricksPreCheck skips TestAccDatabricks unless every
// P0_DATABRICKS_* variable is set, as well as what testAccPreCheck needs. The
// stub's defaults name nothing real, so they can't install against P0.
func testAccDatabricksPreCheck(t *testing.T) {
	testAccPreCheck(t)
	var missing []string
	for name := range databricksAccEnv(&databricksTestInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the Databricks acceptance tests", strings.Join(missing, ", "))
	}
}

// databricksAccInstall reads what TestAccDatabricks installs from the
// P0_DATABRICKS_* variables.
func databricksAccInstall() databricksTestInstall {
	var install databricksTestInstall
	for name, value := range databricksAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return install
}

// Each configuration installs one more component than the last, staging the
// connector and the account before installing them, as the examples do.

func (d databricksTestInstall) connectorStaged() string {
	return fmt.Sprintf(`
resource "p0_databricks_connector_staged" "test" {
  id             = %q
  region         = %q
  domain_pattern = %q
}
`, d.awsAccountId, d.region, d.domainPattern)
}

func (d databricksTestInstall) connector() string {
	return d.connectorStaged() + `
resource "p0_databricks_connector" "test" {
  id             = p0_databricks_connector_staged.test.id
  region         = p0_databricks_connector_staged.test.region
  domain_pattern = p0_databricks_connector_staged.test.domain_pattern
}
`
}

func (d databricksTestInstall) accountStaged() string {
	return d.connector() + fmt.Sprintf(`
resource "p0_databricks_account_staged" "test" {
  id           = %q
  connector    = p0_databricks_connector.test.id
  accounts_url = %q
}
`, d.accountId, d.accountsUrl)
}

func (d databricksTestInstall) account() string {
	return d.accountStaged() + fmt.Sprintf(`
resource "p0_databricks_account" "test" {
  id             = p0_databricks_account_staged.test.id
  connector      = p0_databricks_account_staged.test.connector
  accounts_url   = p0_databricks_account_staged.test.accounts_url
  application_id = %q
}
`, d.applicationId)
}

func (d databricksTestInstall) workspace() string {
	return d.account() + fmt.Sprintf(`
resource "p0_databricks_workspace" "test" {
  id      = %q
  account = p0_databricks_account.test.id
}
`, d.workspaceId)
}

func (d databricksTestInstall) catalogConfig() string {
	return d.workspace() + fmt.Sprintf(`
resource "p0_databricks_catalog" "test" {
  workspace_id = p0_databricks_workspace.test.id
  catalog_name = %q
}
`, d.catalog)
}

// One resource's test: install everything up to it, check its attributes,
// import it, and check that destroying the configuration deletes every item.
type databricksCase struct {
	name     string
	resource string
	config   func(databricksTestInstall) string
	want     func(databricksTestInstall) map[string]string
}

var databricksCases = []databricksCase{
	{
		name:     "ConnectorStaged",
		resource: "p0_databricks_connector_staged.test",
		config:   databricksTestInstall.connectorStaged,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "domain_pattern": d.domainPattern, "federation_audience": "databricks", "state": "stage"}
		},
	},
	{
		name:     "Connector",
		resource: "p0_databricks_connector.test",
		config:   databricksTestInstall.connector,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "domain_pattern": d.domainPattern, "federation_audience": "databricks", "state": "installed"}
		},
	},
	{
		name:     "AccountStaged",
		resource: "p0_databricks_account_staged.test",
		config:   databricksTestInstall.accountStaged,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "federation_audience": "databricks", "state": "stage"}
		},
	},
	{
		name:     "Account",
		resource: "p0_databricks_account.test",
		config:   databricksTestInstall.account,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "application_id": d.applicationId, "federation_audience": "databricks", "state": "installed"}
		},
	},
	{
		name:     "Workspace",
		resource: "p0_databricks_workspace.test",
		config:   databricksTestInstall.workspace,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.workspaceId, "account": d.accountId, "state": "installed"}
		},
	},
	{
		name:     "Catalog",
		resource: "p0_databricks_catalog.test",
		config:   databricksTestInstall.catalogConfig,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.catalog + "@" + d.workspaceId, "workspace_id": d.workspaceId, "catalog_name": d.catalog, "state": "installed"}
		},
	},
}

func (c databricksCase) testCase(provider string, client *internal.P0ProviderData, install databricksTestInstall, preCheck func()) resource.TestCase {
	var checks []resource.TestCheckFunc
	for attribute, value := range c.want(install) {
		checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
	}
	config := provider + c.config(install)

	return resource.TestCase{
		PreCheck:                 preCheck,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(client),
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

var databricksComponentByResourceType = map[string]string{
	"p0_databricks_connector_staged": "connector",
	"p0_databricks_connector":        "connector",
	"p0_databricks_account_staged":   "account",
	"p0_databricks_account":          "account",
	"p0_databricks_workspace":        "workspace",
	"p0_databricks_catalog":          "catalog",
}

func databricksItemPath(component, id string) string {
	return fmt.Sprintf("integrations/databricks/config/%s/%s", component, id)
}

// checkDatabricksItemState checks that P0 has the item in the given state.
func checkDatabricksItemState(client *internal.P0ProviderData, component, id, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		var api struct {
			Item map[string]any `json:"item"`
		}
		if _, err := client.Get(databricksItemPath(component, id), &api); err != nil {
			return fmt.Errorf("could not read the Databricks %s %s: %w", component, id, err)
		}
		if got := api.Item["state"]; got != want {
			return fmt.Errorf("the Databricks %s %s is in state %v, want %s", component, id, got, want)
		}
		return nil
	}
}

// checkDatabricksDestroyed checks that P0 no longer has the item behind any
// Databricks resource in the destroyed state.
func checkDatabricksDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := databricksComponentByResourceType[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(databricksItemPath(component, rs.Primary.ID), &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the Databricks %s %s was deleted: %w", component, rs.Primary.ID, err)
			}
			return fmt.Errorf("the Databricks %s %s is still installed in P0", component, rs.Primary.ID)
		}
		return nil
	}
}

// TestAccDatabricks installs each Databricks resource in a P0 organization (see
// testAccDatabricksPreCheck). The organization must run a P0 version with the
// Databricks integration, and have it enabled.
func TestAccDatabricks(t *testing.T) {
	for _, c := range databricksCases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, c.testCase(testAccProviderConfig(), testAccClient(), databricksAccInstall(), func() { testAccDatabricksPreCheck(t) }))
		})
	}
}

// TestDatabricks installs each Databricks resource against the install API
// stub, so it needs neither TF_ACC nor a P0 organization.
func TestDatabricks(t *testing.T) {
	for _, c := range databricksCases {
		t.Run(c.name, func(t *testing.T) {
			provider, client := newInstallApiStub(t, databricksStubSpec)
			resource.UnitTest(t, c.testCase(provider, client, databricksTestDefaults, nil))
		})
	}
}

// P0 refuses to change a connector's domain pattern after staging, so a new
// pattern must replace the connector rather than update it.
func TestDatabricksConnectorReplacedOnNewDomainPattern(t *testing.T) {
	provider, client := newInstallApiStub(t, databricksStubSpec)
	changed := databricksTestDefaults
	changed.domainPattern = `(.+\.)?example\.org`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(client),
		Steps: []resource.TestStep{
			{Config: provider + databricksTestDefaults.connector()},
			{
				Config: provider + changed.connector(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_connector_staged.test", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_databricks_connector.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_connector.test", "domain_pattern", changed.domainPattern),
					resource.TestCheckResourceAttr("p0_databricks_connector.test", "state", "installed"),
				),
			},
		},
	})
}

// P0 accepts a new application ID when it verifies and configures an account,
// so a recreated service principal updates the account in place.
func TestDatabricksAccountUpdatedOnNewApplicationId(t *testing.T) {
	provider, client := newInstallApiStub(t, databricksStubSpec)
	changed := databricksTestDefaults
	changed.applicationId = "0d26daa6-5e44-4c97-a497-ef015f91254a"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(client),
		Steps: []resource.TestStep{
			{Config: provider + databricksTestDefaults.account()},
			{
				Config: provider + changed.account(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_account.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_account.test", "application_id", changed.applicationId),
					resource.TestCheckResourceAttr("p0_databricks_account.test", "state", "installed"),
				),
			},
		},
	})
}

// Destroying an installed resource returns its item to the "stage" state
// instead of deleting it, so that its staged resource still has an item to read
// and delete.
func TestDatabricksDestroyInstalledKeepsStagedItem(t *testing.T) {
	d := databricksTestDefaults
	cases := []struct {
		name, installed, staged, component, id string
		full, stagedOnly                       func(databricksTestInstall) string
	}{
		{"Connector", "p0_databricks_connector.test", "p0_databricks_connector_staged.test", "connector", d.awsAccountId, databricksTestInstall.connector, databricksTestInstall.connectorStaged},
		{"Account", "p0_databricks_account.test", "p0_databricks_account_staged.test", "account", d.accountId, databricksTestInstall.account, databricksTestInstall.accountStaged},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider, client := newInstallApiStub(t, databricksStubSpec)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkDatabricksDestroyed(client),
				Steps: []resource.TestStep{
					{Config: provider + c.full(d)},
					{
						Config: provider + c.stagedOnly(d),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(c.installed, plancheck.ResourceActionDestroy),
								plancheck.ExpectResourceAction(c.staged, plancheck.ResourceActionNoop),
							},
						},
						Check: checkDatabricksItemState(client, c.component, c.id, "stage"),
					},
					{
						Config: provider + c.stagedOnly(d),
						Check:  resource.TestCheckResourceAttr(c.staged, "state", "stage"),
					},
				},
			})
		})
	}
}

// An update that fails at the configure step leaves P0 with the new values and
// the item in "configure". The next plan must install the account again rather
// than show no changes.
func TestDatabricksAccountReinstalledWhenNotInstalled(t *testing.T) {
	provider, client := newInstallApiStub(t, databricksStubSpec)
	d := databricksTestDefaults

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(client),
		Steps: []resource.TestStep{
			{Config: provider + d.account()},
			{
				// What a failed update leaves: verify saved a new application
				// ID, and configure never ran.
				PreConfig: func() {
					body := map[string]any{"applicationId": "0d26daa6-5e44-4c97-a497-ef015f91254a"}
					if _, err := client.Post(databricksItemPath("account", d.accountId)+"/verify", body, &map[string]any{}); err != nil {
						t.Fatalf("could not verify the account: %s", err)
					}
				},
				Config: provider + d.account(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_account.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_account.test", "application_id", d.applicationId),
					resource.TestCheckResourceAttr("p0_databricks_account.test", "state", "installed"),
				),
			},
		},
	})
}

// A catalog imports by its key, and refuses an import ID that isn't one.
func TestDatabricksCatalogImportId(t *testing.T) {
	provider, client := newInstallApiStub(t, databricksStubSpec)
	d := databricksTestDefaults

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(client),
		Steps: []resource.TestStep{
			{Config: provider + d.catalogConfig()},
			{
				Config:            provider + d.catalogConfig(),
				ResourceName:      "p0_databricks_catalog.test",
				ImportState:       true,
				ImportStateId:     d.catalog + "@" + d.workspaceId,
				ImportStateVerify: true,
			},
			{
				Config:        provider + d.catalogConfig(),
				ResourceName:  "p0_databricks_catalog.test",
				ImportState:   true,
				ImportStateId: d.catalog,
				ExpectError:   regexp.MustCompile(`Import a catalog by <catalog name>@<workspace ID>`),
			},
		},
	})
}

// The schema validators reject malformed values at plan time, before the
// provider calls P0. Each message is one that only the validator produces, so
// a case fails if its validator is deleted.
func TestDatabricksValidators(t *testing.T) {
	provider, _ := newInstallApiStub(t, databricksStubSpec)

	invalid := func(change func(*databricksTestInstall), config func(databricksTestInstall) string, message string) resource.TestStep {
		install := databricksTestDefaults
		change(&install)
		return resource.TestStep{
			Config:      provider + config(install),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(message),
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			invalid(func(d *databricksTestInstall) { d.awsAccountId = "12345" }, databricksTestInstall.connectorStaged, "AWS account IDs should consist of 12 numeric digits"),
			invalid(func(d *databricksTestInstall) { d.region = "us-west" }, databricksTestInstall.connectorStaged, "The connector runs only in commercial AWS regions"),
			invalid(func(d *databricksTestInstall) { d.region = "us-gov-west-1" }, databricksTestInstall.connectorStaged, "The connector runs only in commercial AWS regions"),
			invalid(func(d *databricksTestInstall) { d.domainPattern = "" }, databricksTestInstall.connectorStaged, "string length must be at least 1"),
			invalid(func(d *databricksTestInstall) { d.accountId = "my-account" }, databricksTestInstall.accountStaged, "Databricks account IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.accountsUrl = "https://dbc-1234abcd-5678.cloud.databricks.com" }, databricksTestInstall.accountStaged, "value must be one of"),
			// The application IDs that the app's validator rejects.
			invalid(func(d *databricksTestInstall) { d.applicationId = "" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.applicationId = "p0-connector" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.applicationId = "8c5e2e0a8f0d4a3e9d613b2f4c7a1e05" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.workspaceId = "dbc-1234abcd-5678" }, databricksTestInstall.workspace, "Databricks workspace IDs are numeric"),
			// The catalog names that the app's validator rejects.
			invalid(func(d *databricksTestInstall) { d.catalog = "main.default" }, databricksTestInstall.catalogConfig, "Catalog names have at most 255 characters"),
			invalid(func(d *databricksTestInstall) { d.catalog = "Sales" }, databricksTestInstall.catalogConfig, "Unity Catalog stores catalog names in lowercase"),
			invalid(func(d *databricksTestInstall) { d.catalog = "sales#eu" }, databricksTestInstall.catalogConfig, "P0 can't install a catalog whose name contains"),
			invalid(func(d *databricksTestInstall) { d.catalog = "sales?eu" }, databricksTestInstall.catalogConfig, "P0 can't install a catalog whose name contains"),
			invalid(func(d *databricksTestInstall) { d.catalog = "a%62c" }, databricksTestInstall.catalogConfig, "P0 can't install a catalog whose name contains"),
		},
	})
}
