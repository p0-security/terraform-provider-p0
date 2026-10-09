// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
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

// Each Databricks component's install schema, as the app defines it in
// packages/integrations/databricks/shared/src/components.ts: its fields, and
// those marked `step: "new"`, which P0 refuses to change after staging.
var databricksSchema = map[string]struct{ fields, stepNew []string }{
	"connector": {fields: []string{"region", "domainPattern"}, stepNew: []string{"region", "domainPattern"}},
	"account":   {fields: []string{"connector", "accountsUrl", "applicationId"}, stepNew: []string{"connector", "accountsUrl"}},
	"workspace": {fields: []string{"account"}, stepNew: []string{"account"}},
	"catalog":   {fields: []string{"workspace"}, stepNew: []string{"workspace"}},
}

// newDatabricksFake fakes P0's install API for Databricks items, which P0 stores
// and checks as install-api.ts and configure.ts in the app do. It trims each
// field, drops any field outside the component's schema, so that a field the
// provider misnames reads back empty, and refuses a change to a `step: "new"`
// field after staging. It runs no install checks, so it can't tell whether the
// AWS or Databricks side of an install exists. A test fails a check with the
// fake's refuse hook.
func newDatabricksFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		schema, ok := databricksSchema[key.component]
		if key.integration != "databricks" || !ok {
			t.Errorf("unexpected item %v in the fake Databricks install", key)
			return
		}
		for field, value := range item {
			switch {
			case field == "state" || field == "label":
			case !slices.Contains(schema.fields, field):
				delete(item, field)
			default:
				if text, ok := value.(string); ok {
					item[field] = strings.TrimSpace(text)
				}
			}
		}
	}
	f.check = func(key fakeItemKey, nextState string, previous, updated map[string]any) error {
		if nextState == "stage" {
			return nil
		}
		for _, field := range databricksSchema[key.component].stepNew {
			if !reflect.DeepEqual(updated[field], previous[field]) {
				return fmt.Errorf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
			}
		}
		return nil
	}
	return f
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
	catalogName   string
}

// What the tests against the fake install. They name nothing real.
var databricksTestDefaults = databricksTestInstall{
	awsAccountId:  "123456789012",
	region:        "us-west-2",
	domainPattern: `example\.com`,
	accountId:     "01234567-89ab-cdef-0123-456789abcdef",
	accountsUrl:   "https://accounts.cloud.databricks.com",
	applicationId: "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05",
	workspaceId:   "1234567890123456",
	catalogName:   "p0_acceptance_test",
}

// The application ID of a service principal that replaces the account's.
const databricksNewApplicationId = "0d26daa6-5e44-4c97-a497-ef015f91254a"

func (d databricksTestInstall) catalogKey() string {
	return d.catalogName + "@" + d.workspaceId
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
		"P0_DATABRICKS_CATALOG":        &install.catalogName,
	}
}

// testAccDatabricksPreCheck skips TestAccDatabricks unless every
// P0_DATABRICKS_* variable is set, as well as what testAccPreCheck needs. The
// fake's defaults name nothing real, so they can't install against P0.
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

func (d databricksTestInstall) catalog() string {
	return d.workspace() + fmt.Sprintf(`
resource "p0_databricks_catalog" "test" {
  workspace_id = p0_databricks_workspace.test.id
  catalog_name = %q
}
`, d.catalogName)
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
		config:   databricksTestInstall.catalog,
		want: func(d databricksTestInstall) map[string]string {
			return map[string]string{"id": d.catalogKey(), "workspace_id": d.workspaceId, "catalog_name": d.catalogName, "state": "installed"}
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

// TestDatabricks installs each Databricks resource against the fake, so it
// needs neither TF_ACC nor a P0 organization.
func TestDatabricks(t *testing.T) {
	for _, c := range databricksCases {
		t.Run(c.name, func(t *testing.T) {
			f := newDatabricksFake(t)
			resource.UnitTest(t, c.testCase(providerConfig(f), f.client(), databricksTestDefaults, nil))
		})
	}
}

// P0 refuses to change a connector's domain pattern after staging, so a new
// pattern must replace the connector rather than update it.
func TestDatabricksConnectorReplacedOnNewDomainPattern(t *testing.T) {
	f := newDatabricksFake(t)
	changed := databricksTestDefaults
	changed.domainPattern = `(.+\.)?example\.org`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: providerConfig(f) + databricksTestDefaults.connector()},
			{
				Config: providerConfig(f) + changed.connector(),
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

// P0 accepts a new application ID when it configures an account, so a
// recreated service principal updates the account in place.
func TestDatabricksAccountUpdatedOnNewApplicationId(t *testing.T) {
	f := newDatabricksFake(t)
	changed := databricksTestDefaults
	changed.applicationId = databricksNewApplicationId

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: providerConfig(f) + databricksTestDefaults.account()},
			{
				Config: providerConfig(f) + changed.account(),
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

// P0 checks a new application ID on the configure step alone, which saves
// nothing when the check fails. So a failed update leaves the account installed
// with its current ID, which grants and revokes in it keep using, and the next
// plan shows the update again.
func TestDatabricksAccountKeptWhenNewApplicationIdFails(t *testing.T) {
	f := newDatabricksFake(t)
	d := databricksTestDefaults
	changed := d
	changed.applicationId = databricksNewApplicationId
	// The new service principal can't sign in yet, for example because its
	// federation policy isn't applied, so P0's check on the configure step fails.
	var refuseConfigure atomic.Bool
	f.refuse = func(key fakeItemKey, nextState string) bool {
		return refuseConfigure.Load() && key.component == "account" && nextState == "installed"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: providerConfig(f) + d.account()},
			{
				PreConfig:   func() { refuseConfigure.Store(true) },
				Config:      providerConfig(f) + changed.account(),
				ExpectError: regexp.MustCompile(`Databricks install check failed`),
			},
			{
				PreConfig: func() {
					refuseConfigure.Store(false)
					if item := f.item("databricks", "account", d.accountId); item["state"] != "installed" || item["applicationId"] != d.applicationId {
						t.Errorf("after the failed update, P0 has the account %v, want it installed with application ID %s", item, d.applicationId)
					}
				},
				Config: providerConfig(f) + changed.account(),
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
		{
			name:       "Connector",
			installed:  "p0_databricks_connector.test",
			staged:     "p0_databricks_connector_staged.test",
			component:  "connector",
			id:         d.awsAccountId,
			full:       databricksTestInstall.connector,
			stagedOnly: databricksTestInstall.connectorStaged,
		},
		{
			name:       "Account",
			installed:  "p0_databricks_account.test",
			staged:     "p0_databricks_account_staged.test",
			component:  "account",
			id:         d.accountId,
			full:       databricksTestInstall.account,
			stagedOnly: databricksTestInstall.accountStaged,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newDatabricksFake(t)
			client := f.client()
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkDatabricksDestroyed(client),
				Steps: []resource.TestStep{
					{Config: providerConfig(f) + c.full(d)},
					{
						Config: providerConfig(f) + c.stagedOnly(d),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(c.installed, plancheck.ResourceActionDestroy),
								plancheck.ExpectResourceAction(c.staged, plancheck.ResourceActionNoop),
							},
						},
						Check: checkDatabricksItemState(client, c.component, c.id, "stage"),
					},
					{
						Config: providerConfig(f) + c.stagedOnly(d),
						Check:  resource.TestCheckResourceAttr(c.staged, "state", "stage"),
					},
				},
			})
		})
	}
}

// An account that P0 has at configure plans an update, even though the
// configuration hasn't changed, and applying it finishes the install.
// Otherwise the plan would show no changes while P0 refuses every grant and
// revoke through the account.
func TestDatabricksAccountReinstalledWhenNotInstalled(t *testing.T) {
	f := newDatabricksFake(t)
	d := databricksTestDefaults

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: providerConfig(f) + d.account()},
			{
				PreConfig: func() {
					f.update("databricks", "account", d.accountId, func(item map[string]any) {
						item["state"] = "configure"
					})
				},
				Config: providerConfig(f) + d.account(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_account.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_databricks_account.test", "application_id", d.applicationId),
					resource.TestCheckResourceAttr("p0_databricks_account.test", "state", "installed"),
					checkDatabricksItemState(f.client(), "account", d.accountId, "installed"),
				),
			},
		},
	})
}

// P0 may have an item that it hasn't finished installing, such as one that
// someone started in the P0 app. Importing it works, and the next apply
// finishes the install from the step that P0 has the item at.
func TestDatabricksImportUnfinished(t *testing.T) {
	d := databricksTestDefaults
	cases := []struct {
		name, resource, component, id string
		// The item that P0 has, and the state its install reached.
		item  map[string]any
		state string
		// The installed resource alone, as the item's importer would write it.
		config string
		// Destroying an installed connector or account returns its item to stage,
		// for its staged resource to delete.
		rollsBack bool
	}{
		{
			name:      "ConnectorAtStage",
			resource:  "p0_databricks_connector.test",
			component: "connector",
			id:        d.awsAccountId,
			item:      map[string]any{"region": d.region, "domainPattern": d.domainPattern},
			state:     "stage",
			config: fmt.Sprintf(`
resource "p0_databricks_connector" "test" {
  id             = %q
  region         = %q
  domain_pattern = %q
}
`, d.awsAccountId, d.region, d.domainPattern),
			rollsBack: true,
		},
		{
			name:      "AccountAtConfigure",
			resource:  "p0_databricks_account.test",
			component: "account",
			id:        d.accountId,
			item:      map[string]any{"connector": d.awsAccountId, "accountsUrl": d.accountsUrl, "applicationId": d.applicationId},
			state:     "configure",
			config: fmt.Sprintf(`
resource "p0_databricks_account" "test" {
  id             = %q
  connector      = %q
  accounts_url   = %q
  application_id = %q
}
`, d.accountId, d.awsAccountId, d.accountsUrl, d.applicationId),
			rollsBack: true,
		},
		{
			name:      "WorkspaceAtConfigure",
			resource:  "p0_databricks_workspace.test",
			component: "workspace",
			id:        d.workspaceId,
			item:      map[string]any{"account": d.accountId},
			state:     "configure",
			config: fmt.Sprintf(`
resource "p0_databricks_workspace" "test" {
  id      = %q
  account = %q
}
`, d.workspaceId, d.accountId),
		},
		{
			name:      "CatalogAtStage",
			resource:  "p0_databricks_catalog.test",
			component: "catalog",
			id:        d.catalogKey(),
			item:      map[string]any{"workspace": d.workspaceId},
			state:     "stage",
			config: fmt.Sprintf(`
resource "p0_databricks_catalog" "test" {
  workspace_id = %q
  catalog_name = %q
}
`, d.workspaceId, d.catalogName),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newDatabricksFake(t)
			client := f.client()
			checkDestroy := checkDatabricksDestroyed(client)
			if c.rollsBack {
				checkDestroy = checkDatabricksItemState(client, c.component, c.id, "stage")
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkDestroy,
				Steps: []resource.TestStep{
					{
						PreConfig:          func() { startDatabricksInstall(t, client, c.component, c.id, c.item, c.state) },
						Config:             providerConfig(f) + c.config,
						ResourceName:       c.resource,
						ImportState:        true,
						ImportStateId:      c.id,
						ImportStatePersist: true,
						ImportStateCheck: func(states []*terraform.InstanceState) error {
							if len(states) != 1 || states[0].Attributes["state"] != c.state {
								return fmt.Errorf("imported %v, want one item at %s", states, c.state)
							}
							return nil
						},
					},
					{
						Config: providerConfig(f) + c.config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(c.resource, plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(c.resource, "state", "installed"),
							checkDatabricksItemState(client, c.component, c.id, "installed"),
						),
					},
				},
			})
		})
	}
}

// Starts an install the way the P0 app does: stages the item, and verifies it
// too for an install that reached configure.
func startDatabricksInstall(t *testing.T, client *internal.P0ProviderData, component, id string, item map[string]any, state string) {
	t.Helper()
	if _, err := client.Post("integrations/databricks/config", struct{}{}, &map[string]any{}); err != nil {
		t.Fatalf("could not create the Databricks integration: %s", err)
	}
	path := databricksItemPath(component, id)
	if _, err := client.Put(path, item, &map[string]any{}); err != nil {
		t.Fatalf("could not stage the Databricks %s %s: %s", component, id, err)
	}
	if state == "configure" {
		if _, err := client.Post(path+"/verify", item, &map[string]any{}); err != nil {
			t.Fatalf("could not verify the Databricks %s %s: %s", component, id, err)
		}
	}
}

// A create whose install check fails leaves its item staged in P0, and
// Terraform keeps the resource, tainted. Removing its block then deletes the
// item, so a catalog that never installed isn't left in P0. Workspaces and
// catalogs have no staged resource to delete it later.
func TestDatabricksFailedCreateDestroyedWhenRemoved(t *testing.T) {
	f := newDatabricksFake(t)
	d := databricksTestDefaults
	// P0's service principal can't read the catalog, so its verify step fails.
	f.refuse = func(key fakeItemKey, nextState string) bool {
		return key.component == "catalog" && nextState == "configure"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{
				Config:      providerConfig(f) + d.catalog(),
				ExpectError: regexp.MustCompile(`Databricks install check failed`),
			},
			{
				PreConfig: func() {
					if item := f.item("databricks", "catalog", d.catalogKey()); item["state"] != "stage" {
						t.Errorf("after the failed create, P0 has the catalog %v, want it staged", item)
					}
				},
				Config: providerConfig(f) + d.workspace(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_databricks_catalog.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.item("databricks", "catalog", d.catalogKey()); item != nil {
						return fmt.Errorf("P0 still has the catalog that never installed: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// A catalog imports by its key, and refuses an import ID that isn't one, or
// whose catalog name or workspace ID the configuration would refuse. The read
// after an import puts the key in a URL path, where `sales%@7` doesn't parse
// and `sales#eu@7` reads `sales`.
func TestDatabricksCatalogImportId(t *testing.T) {
	f := newDatabricksFake(t)
	d := databricksTestDefaults

	refused := func(id, message string) resource.TestStep {
		return resource.TestStep{
			Config:        providerConfig(f) + d.catalog(),
			ResourceName:  "p0_databricks_catalog.test",
			ImportState:   true,
			ImportStateId: id,
			// Terraform wraps a long message across lines.
			ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(message), " ", `\s+`)),
		}
	}
	const (
		notAKey     = "Import a catalog by <catalog name>@<workspace ID>"
		forbidden   = "Catalog names can't contain a period, a space, a forward slash or a control character"
		urlPathOnly = `P0 can't install a catalog whose name contains #, ?, % or \. Contact support@p0.dev if you need P0 to manage it.`
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkDatabricksDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: providerConfig(f) + d.catalog()},
			{
				Config:            providerConfig(f) + d.catalog(),
				ResourceName:      "p0_databricks_catalog.test",
				ImportState:       true,
				ImportStateId:     d.catalogKey(),
				ImportStateVerify: true,
			},
			refused(d.catalogName, notAKey),
			refused(d.catalogName+"@dbc-1234abcd-5678", notAKey),
			refused("sales%@7", urlPathOnly),
			refused("sales#eu@7", urlPathOnly),
			refused("main.default@7", forbidden),
			refused("tab\there@7", forbidden),
			refused("Sales@7", "Unity Catalog stores catalog names in lowercase, so enter sales"),
		},
	})
}

// The schema validators reject malformed values at plan time, before the
// provider calls P0. Each message is one that only the validator produces, so
// a case fails if its validator is deleted.
func TestDatabricksValidators(t *testing.T) {
	f := newDatabricksFake(t)

	invalid := func(change func(*databricksTestInstall), config func(databricksTestInstall) string, message string) resource.TestStep {
		install := databricksTestDefaults
		change(&install)
		return resource.TestStep{
			Config:   providerConfig(f) + config(install),
			PlanOnly: true,
			// Terraform wraps a long message across lines.
			ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(message), " ", `\s+`)),
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			invalid(func(d *databricksTestInstall) { d.awsAccountId = "12345" }, databricksTestInstall.connectorStaged, "AWS account IDs should consist of 12 numeric digits"),
			invalid(func(d *databricksTestInstall) { d.region = "us-west" }, databricksTestInstall.connectorStaged, "The connector runs only in commercial AWS regions"),
			invalid(func(d *databricksTestInstall) { d.region = "us-gov-west-1" }, databricksTestInstall.connectorStaged, "The connector runs only in commercial AWS regions"),
			invalid(func(d *databricksTestInstall) { d.domainPattern = "" }, databricksTestInstall.connectorStaged, "string length must be at least 1"),
			// P0 trims the pattern, which would then read back different from the
			// configuration.
			invalid(func(d *databricksTestInstall) { d.domainPattern = ` example\.com` }, databricksTestInstall.connectorStaged, "Whitespace around a value"),
			invalid(func(d *databricksTestInstall) { d.domainPattern = "example\\.com\n" }, databricksTestInstall.connectorStaged, "Whitespace around a value"),
			invalid(func(d *databricksTestInstall) { d.accountId = "my-account" }, databricksTestInstall.accountStaged, "Databricks account IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.accountsUrl = "https://dbc-1234abcd-5678.cloud.databricks.com" }, databricksTestInstall.accountStaged, "value must be one of"),
			// The application IDs that the app's validator rejects.
			invalid(func(d *databricksTestInstall) { d.applicationId = "" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.applicationId = "p0-connector" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.applicationId = "8c5e2e0a8f0d4a3e9d613b2f4c7a1e05" }, databricksTestInstall.account, "Application IDs are UUIDs"),
			invalid(func(d *databricksTestInstall) { d.workspaceId = "dbc-1234abcd-5678" }, databricksTestInstall.workspace, "Databricks workspace IDs are numeric"),
			// The catalog names that the app's catalogNameError rejects, in its words.
			invalid(func(d *databricksTestInstall) { d.catalogName = "" }, databricksTestInstall.catalog, "Enter the catalog's name"),
			invalid(func(d *databricksTestInstall) { d.catalogName = strings.Repeat("a", 256) }, databricksTestInstall.catalog, "Catalog names are at most 255 characters"),
			invalid(func(d *databricksTestInstall) { d.catalogName = "main.default" }, databricksTestInstall.catalog, "Catalog names can't contain a period, a space, a forward slash or a control character"),
			invalid(func(d *databricksTestInstall) { d.catalogName = "Sales" }, databricksTestInstall.catalog, "Unity Catalog stores catalog names in lowercase, so enter sales"),
			invalid(func(d *databricksTestInstall) { d.catalogName = "sales#eu" }, databricksTestInstall.catalog, `P0 can't install a catalog whose name contains #, ?, % or \`),
			invalid(func(d *databricksTestInstall) { d.catalogName = "sales?eu" }, databricksTestInstall.catalog, `P0 can't install a catalog whose name contains #, ?, % or \`),
			invalid(func(d *databricksTestInstall) { d.catalogName = "a%62c" }, databricksTestInstall.catalog, `P0 can't install a catalog whose name contains #, ?, % or \`),
		},
	})
}
