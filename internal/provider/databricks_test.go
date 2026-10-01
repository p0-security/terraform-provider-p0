// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
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

// databricksAccInstall reads what the acceptance tests install from the
// P0_DATABRICKS_* variables, falling back to the defaults. Any well-formed
// values install while P0 doesn't verify Databricks installs. Once it does,
// they must name a deployed connector, an account whose service principal it
// federates into, and a workspace and catalog that the principal administers.
func databricksAccInstall() databricksTestInstall {
	install := databricksTestDefaults
	for name, value := range map[string]*string{
		"P0_DATABRICKS_AWS_ACCOUNT_ID": &install.awsAccountId,
		"P0_DATABRICKS_REGION":         &install.region,
		"P0_DATABRICKS_DOMAIN_PATTERN": &install.domainPattern,
		"P0_DATABRICKS_ACCOUNT_ID":     &install.accountId,
		"P0_DATABRICKS_ACCOUNTS_URL":   &install.accountsUrl,
		"P0_DATABRICKS_APPLICATION_ID": &install.applicationId,
		"P0_DATABRICKS_WORKSPACE_ID":   &install.workspaceId,
		"P0_DATABRICKS_CATALOG":        &install.catalog,
	} {
		if env := os.Getenv(name); env != "" {
			*value = env
		}
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
  id        = %q
  workspace = p0_databricks_workspace.test.id
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
			return map[string]string{"id": d.awsAccountId, "region": d.region, "domain_pattern": d.domainPattern, "state": "stage"}
		},
	},
	{
		name:     "Connector",
		resource: "p0_databricks_connector.test",
		config:   databricksTestInstall.connector,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "domain_pattern": d.domainPattern, "state": "installed"}
		},
	},
	{
		name:     "AccountStaged",
		resource: "p0_databricks_account_staged.test",
		config:   databricksTestInstall.accountStaged,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "state": "stage"}
		},
	},
	{
		name:     "Account",
		resource: "p0_databricks_account.test",
		config:   databricksTestInstall.account,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.accountId, "connector": d.awsAccountId, "accounts_url": d.accountsUrl, "application_id": d.applicationId, "state": "installed"}
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
			return map[string]string{"id": d.catalog, "workspace": d.workspaceId, "state": "installed"}
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

// checkDatabricksDestroyed checks that P0 no longer has the item behind any
// Databricks resource in the destroyed state.
func checkDatabricksDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := databricksComponentByResourceType[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(fmt.Sprintf("integrations/databricks/config/%s/%s", component, rs.Primary.ID), &map[string]any{})
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
// testAccPreCheck). The organization must run a P0 version with the Databricks
// integration, and have it enabled.
func TestAccDatabricks(t *testing.T) {
	for _, c := range databricksCases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, c.testCase(testAccProviderConfig(), testAccClient(), databricksAccInstall(), func() { testAccPreCheck(t) }))
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

// The schema validators reject malformed values at plan time, before the
// provider calls P0.
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
			invalid(func(d *databricksTestInstall) { d.region = "us-west" }, databricksTestInstall.connectorStaged, "AWS region should be in the format"),
			invalid(func(d *databricksTestInstall) { d.domainPattern = "" }, databricksTestInstall.connectorStaged, "domain_pattern"),
			invalid(func(d *databricksTestInstall) { d.accountId = "my-account" }, databricksTestInstall.accountStaged, "Databricks account IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.accountsUrl = "https://dbc-1234abcd-5678.cloud.databricks.com" }, databricksTestInstall.accountStaged, "accounts_url"),
			invalid(func(d *databricksTestInstall) { d.applicationId = "p0-connector" }, databricksTestInstall.account, "Service principal application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.workspaceId = "dbc-1234abcd-5678" }, databricksTestInstall.workspace, "Databricks workspace IDs are numeric"),
			invalid(func(d *databricksTestInstall) { d.catalog = "main.default" }, databricksTestInstall.catalogConfig, "Catalog names have at most 255 characters"),
		},
	})
}
