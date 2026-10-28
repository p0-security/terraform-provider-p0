// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/url"
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

// A copy of databricks_ai_test.go, for Redshift (AI).

// What the Redshift (AI) tests install.
type redshiftAiTestInstall struct {
	awsAccountId string
	region       string
	clusterId    string
	clusterType  string
	database     string
	objectOwner  string
}

// What the tests against the fake install. They name nothing real.
var redshiftAiTestDefaults = redshiftAiTestInstall{
	awsAccountId: "123456789012",
	region:       "us-west-2",
	clusterId:    "123456789012/us-west-2/analytics",
	clusterType:  "provisioned",
	database:     "dev",
	objectOwner:  "etl_owner",
}

// The variables that name what TestAccRedshiftAi installs. P0 verifies every
// Redshift (AI) component, so they must name a deployed connector, and a
// cluster in its AWS account that the connector's role and database user can
// reach. The object owner must name an existing database user.
func redshiftAiAccEnv(install *redshiftAiTestInstall) map[string]*string {
	return map[string]*string{
		"P0_REDSHIFT_AI_AWS_ACCOUNT_ID": &install.awsAccountId,
		"P0_REDSHIFT_AI_REGION":         &install.region,
		"P0_REDSHIFT_AI_CLUSTER_ID":     &install.clusterId,
		"P0_REDSHIFT_AI_CLUSTER_TYPE":   &install.clusterType,
		"P0_REDSHIFT_AI_DATABASE":       &install.database,
		"P0_REDSHIFT_AI_OBJECT_OWNER":   &install.objectOwner,
	}
}

// TestAccRedshiftAi installs into the P0 organization P0_ORG, served at
// P0_HOST (the provider's default host unless set), with the API token
// P0_API_TOKEN. Use an organization set aside for tests: they create and
// delete real installs.
const (
	redshiftAiAccOrgEnv   = "P0_ORG"
	redshiftAiAccHostEnv  = "P0_HOST"
	redshiftAiAccTokenEnv = "P0_API_TOKEN"
	// The host that the provider calls unless its `host` says otherwise.
	redshiftAiAccDefaultHost = "https://api.p0.app"
)

// testAccRedshiftAiPreCheck skips TestAccRedshiftAi unless it has a P0
// organization to run against and every P0_REDSHIFT_AI_* variable is set. It
// skips rather than fails, because CI sets TF_ACC without an organization. The
// fake's defaults name nothing real, so they can't install against P0.
func testAccRedshiftAiPreCheck(t *testing.T) {
	if os.Getenv(redshiftAiAccOrgEnv) == "" || os.Getenv(redshiftAiAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the Redshift (AI) acceptance tests against a P0 organization", redshiftAiAccOrgEnv, redshiftAiAccTokenEnv)
	}
	var missing []string
	for name := range redshiftAiAccEnv(&redshiftAiTestInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the Redshift (AI) acceptance tests", strings.Join(missing, ", "))
	}
}

func redshiftAiAccHost() string {
	if host := os.Getenv(redshiftAiAccHostEnv); host != "" {
		return host
	}
	return redshiftAiAccDefaultHost
}

// redshiftAiAccProviderConfig configures the provider for the acceptance-test
// organization. The provider reads the API token from P0_API_TOKEN itself.
func redshiftAiAccProviderConfig() string {
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}
`, os.Getenv(redshiftAiAccOrgEnv), redshiftAiAccHost())
}

// redshiftAiAccClient calls the acceptance-test organization's API directly,
// for checks that Terraform can't make, such as that a destroyed install is
// gone.
func redshiftAiAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        redshiftAiOrgUrl(redshiftAiAccHost(), os.Getenv(redshiftAiAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(redshiftAiAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// redshiftAiAccInstall reads what TestAccRedshiftAi installs from the
// P0_REDSHIFT_AI_* variables.
func redshiftAiAccInstall() redshiftAiTestInstall {
	var install redshiftAiTestInstall
	for name, value := range redshiftAiAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return install
}

// Each configuration installs one more component than the last, staging the
// connector and the cluster before installing them, as the examples do.

func (d redshiftAiTestInstall) connectorStaged() string {
	return fmt.Sprintf(`
resource "p0_redshift_ai_connector_staged" "test" {
  id     = %q
  region = %q
}
`, d.awsAccountId, d.region)
}

func (d redshiftAiTestInstall) connector() string {
	return d.connectorStaged() + `
resource "p0_redshift_ai_connector" "test" {
  id     = p0_redshift_ai_connector_staged.test.id
  region = p0_redshift_ai_connector_staged.test.region
}
`
}

func (d redshiftAiTestInstall) clusterStaged() string {
	return d.connector() + fmt.Sprintf(`
resource "p0_redshift_ai_cluster_staged" "test" {
  id        = %q
  connector = p0_redshift_ai_connector.test.id
  type      = %q
}
`, d.clusterId, d.clusterType)
}

// Leaves object_owner out when the install has none.
func (d redshiftAiTestInstall) cluster() string {
	objectOwner := ""
	if d.objectOwner != "" {
		objectOwner = fmt.Sprintf("\n  object_owner = %q", d.objectOwner)
	}
	return d.clusterStaged() + fmt.Sprintf(`
resource "p0_redshift_ai_cluster" "test" {
  id        = p0_redshift_ai_cluster_staged.test.id
  connector = p0_redshift_ai_cluster_staged.test.connector
  type      = p0_redshift_ai_cluster_staged.test.type
  database  = %q%s
}
`, d.database, objectOwner)
}

// One resource's test: install everything up to it, check its attributes,
// import it, and check that destroying the configuration deletes every item.
type redshiftAiCase struct {
	name     string
	resource string
	config   func(redshiftAiTestInstall) string
	want     func(redshiftAiTestInstall) map[string]string
	// What only the fake can check, such as the ARN it sets.
	wantFake func(redshiftAiTestInstall) map[string]string
}

var redshiftAiCases = []redshiftAiCase{
	{
		name:     "ConnectorStaged",
		resource: "p0_redshift_ai_connector_staged.test",
		config:   redshiftAiTestInstall.connectorStaged,
		want: func(d redshiftAiTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "state": "stage"}
		},
	},
	{
		name:     "Connector",
		resource: "p0_redshift_ai_connector.test",
		config:   redshiftAiTestInstall.connector,
		want: func(d redshiftAiTestInstall) map[string]string {
			return map[string]string{"id": d.awsAccountId, "region": d.region, "state": "installed"}
		},
	},
	{
		name:     "ClusterStaged",
		resource: "p0_redshift_ai_cluster_staged.test",
		config:   redshiftAiTestInstall.clusterStaged,
		want: func(d redshiftAiTestInstall) map[string]string {
			return map[string]string{"id": d.clusterId, "connector": d.awsAccountId, "type": d.clusterType, "state": "stage"}
		},
	},
	{
		name:     "Cluster",
		resource: "p0_redshift_ai_cluster.test",
		config:   redshiftAiTestInstall.cluster,
		want: func(d redshiftAiTestInstall) map[string]string {
			return map[string]string{"id": d.clusterId, "connector": d.awsAccountId, "type": d.clusterType, "database": d.database, "object_owner": d.objectOwner, "state": "installed"}
		},
		wantFake: func(d redshiftAiTestInstall) map[string]string {
			return map[string]string{"arn": redshiftAiArn(d.clusterId)}
		},
	},
}

func (c redshiftAiCase) testCase(provider string, client *internal.P0ProviderData, install redshiftAiTestInstall, preCheck func(), fake bool) resource.TestCase {
	var checks []resource.TestCheckFunc
	for attribute, value := range c.want(install) {
		checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
	}
	if fake && c.wantFake != nil {
		for attribute, value := range c.wantFake(install) {
			checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
		}
	}
	config := provider + c.config(install)

	return resource.TestCase{
		PreCheck:                 preCheck,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkRedshiftAiDestroyed(client),
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

var redshiftAiComponentByResourceType = map[string]string{
	"p0_redshift_ai_connector_staged": "connector",
	"p0_redshift_ai_connector":        "connector",
	"p0_redshift_ai_cluster_staged":   "cluster",
	"p0_redshift_ai_cluster":          "cluster",
}

// A cluster's key holds slashes, which the provider escapes in the path.
func redshiftAiItemPath(component, id string) string {
	return fmt.Sprintf("integrations/redshift-ai/config/%s/%s", component, url.PathEscape(id))
}

// checkRedshiftAiDestroyed checks that P0 no longer has the item behind any
// Redshift (AI) resource in the destroyed state.
func checkRedshiftAiDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			component, ok := redshiftAiComponentByResourceType[rs.Type]
			if !ok {
				continue
			}
			resp, err := client.Get(redshiftAiItemPath(component, rs.Primary.ID), &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the Redshift (AI) %s %s was deleted: %w", component, rs.Primary.ID, err)
			}
			return fmt.Errorf("the Redshift (AI) %s %s is still installed in P0", component, rs.Primary.ID)
		}
		return nil
	}
}

// TestAccRedshiftAi installs each Redshift (AI) resource in a P0 organization
// (see testAccRedshiftAiPreCheck). The organization must run a P0 version
// with the Redshift (AI) integration.
func TestAccRedshiftAi(t *testing.T) {
	for _, c := range redshiftAiCases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, c.testCase(redshiftAiAccProviderConfig(), redshiftAiAccClient(), redshiftAiAccInstall(), func() { testAccRedshiftAiPreCheck(t) }, false))
		})
	}
}

// TestRedshiftAi installs each Redshift (AI) resource against the fake, so it
// needs neither TF_ACC nor a P0 organization.
func TestRedshiftAi(t *testing.T) {
	for _, c := range redshiftAiCases {
		t.Run(c.name, func(t *testing.T) {
			f := newRedshiftAiFake(t)
			resource.UnitTest(t, c.testCase(f.providerConfig(), f.client(), redshiftAiTestDefaults, nil, true))
		})
	}
}

// A Serverless workgroup with no object owner installs as a cluster does, and
// its object owner reads back unset.
func TestRedshiftAiServerlessWithoutObjectOwner(t *testing.T) {
	f := newRedshiftAiFake(t)
	d := redshiftAiTestDefaults
	d.clusterId = "123456789012/us-west-2/agents"
	d.clusterType = "serverless"
	d.objectOwner = ""

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkRedshiftAiDestroyed(f.client()),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + d.cluster(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "type", "serverless"),
					resource.TestCheckNoResourceAttr("p0_redshift_ai_cluster.test", "object_owner"),
					resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "arn", redshiftAiArn(d.clusterId)),
					resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "state", "installed"),
				),
			},
			{
				Config:            f.providerConfig() + d.cluster(),
				ResourceName:      "p0_redshift_ai_cluster.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// P0 accepts a new database or object owner when it configures a cluster, so
// either updates the cluster in place. Removing the object owner clears it in
// P0, so the connector's own user takes over what agents created.
func TestRedshiftAiClusterUpdatedInPlace(t *testing.T) {
	d := redshiftAiTestDefaults
	cases := []struct {
		name, attribute, want string
		change                func(*redshiftAiTestInstall)
	}{
		{name: "Database", attribute: "database", want: "analytics", change: func(d *redshiftAiTestInstall) { d.database = "analytics" }},
		{name: "ObjectOwner", attribute: "object_owner", want: "IAMR:etl-role", change: func(d *redshiftAiTestInstall) { d.objectOwner = "IAMR:etl-role" }},
		{name: "ObjectOwnerRemoved", attribute: "object_owner", change: func(d *redshiftAiTestInstall) { d.objectOwner = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRedshiftAiFake(t)
			changed := d
			c.change(&changed)
			attribute := resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", c.attribute, c.want)
			if c.want == "" {
				attribute = resource.TestCheckNoResourceAttr("p0_redshift_ai_cluster.test", c.attribute)
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkRedshiftAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{Config: f.providerConfig() + d.cluster()},
					{
						Config: f.providerConfig() + changed.cluster(),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("p0_redshift_ai_cluster.test", plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							attribute,
							resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "arn", redshiftAiArn(d.clusterId)),
							resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "state", "installed"),
							func(*terraform.State) error {
								if item := f.item("redshift-ai", "cluster", d.clusterId); item["objectOwner"] != changed.objectOwner {
									return fmt.Errorf("P0 has the cluster %v, want objectOwner %q", item, changed.objectOwner)
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

// P0 checks a new database or object owner on the configure step alone, which
// saves nothing when the check fails. So a failed update leaves the cluster
// installed with its current value, which grants and revokes in it keep
// using, and the next plan shows the update again.
func TestRedshiftAiClusterKeptWhenUpdateFails(t *testing.T) {
	d := redshiftAiTestDefaults
	cases := []struct {
		name, attribute, field, current, want string
		change                                func(*redshiftAiTestInstall)
	}{
		// The connector's user can't connect to the new database.
		{
			name:      "Database",
			attribute: "database",
			field:     "database",
			current:   d.database,
			want:      "analytics",
			change:    func(d *redshiftAiTestInstall) { d.database = "analytics" },
		},
		// The new object owner names no user.
		{
			name:      "ObjectOwner",
			attribute: "object_owner",
			field:     "objectOwner",
			current:   d.objectOwner,
			want:      "data_owner",
			change:    func(d *redshiftAiTestInstall) { d.objectOwner = "data_owner" },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRedshiftAiFake(t)
			changed := d
			c.change(&changed)
			var refuseConfigure atomic.Bool
			f.refuse = func(key fakeItemKey, nextState string) bool {
				return refuseConfigure.Load() && key.component == "cluster" && nextState == "installed"
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkRedshiftAiDestroyed(f.client()),
				Steps: []resource.TestStep{
					{Config: f.providerConfig() + d.cluster()},
					{
						PreConfig:   func() { refuseConfigure.Store(true) },
						Config:      f.providerConfig() + changed.cluster(),
						ExpectError: regexp.MustCompile(`Redshift \(AI\) install check failed`),
					},
					{
						PreConfig: func() {
							refuseConfigure.Store(false)
							if item := f.item("redshift-ai", "cluster", d.clusterId); item["state"] != "installed" || item[c.field] != c.current {
								t.Errorf("after the failed update, P0 has the cluster %v, want it installed with %s %s", item, c.field, c.current)
							}
						},
						Config: f.providerConfig() + changed.cluster(),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("p0_redshift_ai_cluster.test", plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", c.attribute, c.want),
							resource.TestCheckResourceAttr("p0_redshift_ai_cluster.test", "state", "installed"),
						),
					},
				},
			})
		})
	}
}

// The provider refuses at plan time what P0 would refuse: a malformed key, a
// cluster outside AWS's commercial partition, an unknown type, and a database
// or object owner that isn't a Redshift name.
func TestRedshiftAiClusterInvalid(t *testing.T) {
	cases := []struct {
		name   string
		change func(*redshiftAiTestInstall)
		want   string
	}{
		{name: "Key", change: func(d *redshiftAiTestInstall) { d.clusterId = "123456789012/analytics" }, want: `Invalid Redshift cluster ID`},
		{name: "UpperCaseName", change: func(d *redshiftAiTestInstall) { d.clusterId = "123456789012/us-west-2/Analytics" }, want: `Invalid Redshift cluster ID`},
		{name: "GovCloud", change: func(d *redshiftAiTestInstall) { d.clusterId = "123456789012/us-gov-west-1/analytics" }, want: `Unsupported AWS region`},
		{name: "Type", change: func(d *redshiftAiTestInstall) { d.clusterType = "dc2" }, want: `value must be one of`},
		{name: "Database", change: func(d *redshiftAiTestInstall) { d.database = "dev db" }, want: `Use a database name`},
		{name: "ObjectOwner", change: func(d *redshiftAiTestInstall) { d.objectOwner = " etl_owner" }, want: `Use a Redshift user name`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRedshiftAiFake(t)
			d := redshiftAiTestDefaults
			c.change(&d)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      f.providerConfig() + d.cluster(),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(c.want),
					},
				},
			})
		})
	}
}
