// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// What the log-source tests install. They name nothing real.
type logSourceTestInstall struct {
	id             string
	accountId      string
	region         string
	logGroupName   string
	coverage       string
	projectId      string
	hostProjectId  string
	withCloudtrail bool
}

var logSourceTestDefaults = logSourceTestInstall{
	id:             "123456789012:us-west-2",
	accountId:      "123456789012",
	region:         "us-west-2",
	logGroupName:   "aws-cloudtrail-logs",
	coverage:       "organization",
	projectId:      "my-project-123",
	hostProjectId:  "p0-connectors",
	withCloudtrail: true,
}

func (d logSourceTestInstall) cloudtrail() string {
	if !d.withCloudtrail {
		return ""
	}
	return fmt.Sprintf(`
  cloudtrail = {
    log_group_name = %q
    coverage       = %q
  }`, d.logGroupName, d.coverage)
}

func (d logSourceTestInstall) awsStaged() string {
	return fmt.Sprintf(`
resource "p0_aws_cloudwatch_logs_staged" "test" {
  id         = %q
  account_id = %q
  region     = %q%s
}
`, d.id, d.accountId, d.region, d.cloudtrail())
}

func (d logSourceTestInstall) aws() string {
	cloudtrail := ""
	if d.withCloudtrail {
		cloudtrail = "\n  cloudtrail = p0_aws_cloudwatch_logs_staged.test.cloudtrail"
	}
	return d.awsStaged() + fmt.Sprintf(`
resource "p0_aws_cloudwatch_logs" "test" {
  id         = p0_aws_cloudwatch_logs_staged.test.id
  account_id = p0_aws_cloudwatch_logs_staged.test.account_id
  region     = p0_aws_cloudwatch_logs_staged.test.region%s
}
`, cloudtrail)
}

func (d logSourceTestInstall) projectStaged() string {
	return fmt.Sprintf(`
resource "p0_gcp_cloud_logging_staged" "test" {
  project_id      = %q
  host_project_id = %q
}
`, d.projectId, d.hostProjectId)
}

func (d logSourceTestInstall) project() string {
	return d.projectStaged() + `
resource "p0_gcp_cloud_logging" "test" {
  project_id      = p0_gcp_cloud_logging_staged.test.project_id
  host_project_id = p0_gcp_cloud_logging_staged.test.host_project_id
}
`
}

func (d logSourceTestInstall) organizationStaged() string {
	return fmt.Sprintf(`
resource "p0_gcp_organization_cloud_logging_staged" "test" {
  host_project_id = %q
}
`, d.hostProjectId)
}

func (d logSourceTestInstall) organization() string {
	return d.organizationStaged() + `
resource "p0_gcp_organization_cloud_logging" "test" {
  host_project_id = p0_gcp_organization_cloud_logging_staged.test.host_project_id
}
`
}

// One resource's test: install everything up to it, check its attributes,
// import it, and check that destroying the configuration deletes every item.
type logSourceCase struct {
	name     string
	resource string
	config   func(logSourceTestInstall) string
	want     func(logSourceTestInstall) map[string]string
	// The attribute that identifies an imported resource, and the import ID.
	importAttribute string
	importId        func(logSourceTestInstall) string
}

var logSourceCases = []logSourceCase{
	{
		name:     "AwsStaged",
		resource: "p0_aws_cloudwatch_logs_staged.test",
		config:   logSourceTestInstall.awsStaged,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"id":                        d.id,
				"account_id":                d.accountId,
				"region":                    d.region,
				"cloudtrail.log_group_name": d.logGroupName,
				"cloudtrail.coverage":       d.coverage,
				"connector_name":            "p0-access-logs",
				"invoke_role_name":          "P0AccessLogsInvoker-" + d.region,
				"invoke_trust_policy":       `{"Version":"2012-10-17","Statement":[]}`,
				"execution_policy":          fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Resource":"%s"}]}`, d.logGroupName),
				"state":                     "stage",
			}
		},
		importAttribute: "id",
		importId:        func(d logSourceTestInstall) string { return d.id },
	},
	{
		name:     "Aws",
		resource: "p0_aws_cloudwatch_logs.test",
		config:   logSourceTestInstall.aws,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"id":                        d.id,
				"account_id":                d.accountId,
				"region":                    d.region,
				"cloudtrail.log_group_name": d.logGroupName,
				"cloudtrail.coverage":       d.coverage,
				"connector_name":            "p0-access-logs",
				"invoke_role_name":          "P0AccessLogsInvoker-" + d.region,
				"state":                     "installed",
			}
		},
		importAttribute: "id",
		importId:        func(d logSourceTestInstall) string { return d.id },
	},
	{
		name:     "GcpProjectStaged",
		resource: "p0_gcp_cloud_logging_staged.test",
		config:   logSourceTestInstall.projectStaged,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"project_id":                d.projectId,
				"host_project_id":           d.hostProjectId,
				"connector_service_name":    "p0-logs-" + d.projectId,
				"connector_service_account": fmt.Sprintf("p0-logs-%s@%s.iam.gserviceaccount.com", d.projectId, d.hostProjectId),
				"region":                    "us-central1",
				"state":                     "stage",
			}
		},
		importAttribute: "project_id",
		importId:        func(d logSourceTestInstall) string { return d.projectId },
	},
	{
		name:     "GcpProject",
		resource: "p0_gcp_cloud_logging.test",
		config:   logSourceTestInstall.project,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"project_id":             d.projectId,
				"host_project_id":        d.hostProjectId,
				"connector_service_name": "p0-logs-" + d.projectId,
				"region":                 "us-central1",
				"connector_service_uri":  logSourceServiceUri(fakeItemKey{"gcp-cloud-logging", "project", d.projectId}),
				"state":                  "installed",
			}
		},
		importAttribute: "project_id",
		importId:        func(d logSourceTestInstall) string { return d.projectId },
	},
	{
		name:     "GcpOrganizationStaged",
		resource: "p0_gcp_organization_cloud_logging_staged.test",
		config:   logSourceTestInstall.organizationStaged,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"host_project_id":           d.hostProjectId,
				"connector_service_name":    "p0-logs-org",
				"connector_service_account": fmt.Sprintf("p0-logs-org@%s.iam.gserviceaccount.com", d.hostProjectId),
				"region":                    "us-central1",
				"state":                     "stage",
			}
		},
		importAttribute: "host_project_id",
		importId:        func(d logSourceTestInstall) string { return d.hostProjectId },
	},
	{
		name:     "GcpOrganization",
		resource: "p0_gcp_organization_cloud_logging.test",
		config:   logSourceTestInstall.organization,
		want: func(d logSourceTestInstall) map[string]string {
			return map[string]string{
				"host_project_id":        d.hostProjectId,
				"connector_service_name": "p0-logs-org",
				"connector_service_uri":  logSourceServiceUri(fakeItemKey{"gcp-cloud-logging", "organization", "_"}),
				"state":                  "installed",
			}
		},
		importAttribute: "host_project_id",
		importId:        func(d logSourceTestInstall) string { return d.hostProjectId },
	},
}

// TestLogSources installs each log-source resource against the fake, so it
// needs neither TF_ACC nor a P0 organization.
func TestLogSources(t *testing.T) {
	for _, c := range logSourceCases {
		t.Run(c.name, func(t *testing.T) {
			f := newLogSourceFake(t)
			d := logSourceTestDefaults
			var checks []resource.TestCheckFunc
			for attribute, value := range c.want(d) {
				checks = append(checks, resource.TestCheckResourceAttr(c.resource, attribute, value))
			}
			config := f.providerConfig() + c.config(d)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkLogSourcesDestroyed(f.client()),
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  resource.ComposeAggregateTestCheckFunc(checks...),
					},
					{
						Config:                               config,
						ResourceName:                         c.resource,
						ImportState:                          true,
						ImportStateId:                        c.importId(d),
						ImportStateVerify:                    true,
						ImportStateVerifyIdentifierAttribute: c.importAttribute,
					},
				},
			})
		})
	}
}

// A log source without a CloudTrail trail is stored with the trail `none`, and
// its cloudtrail reads back unset.
func TestAwsCloudWatchLogsWithoutCloudtrail(t *testing.T) {
	f := newLogSourceFake(t)
	d := logSourceTestDefaults
	d.withCloudtrail = false

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLogSourcesDestroyed(f.client()),
		Steps: []resource.TestStep{
			{
				Config: f.providerConfig() + d.aws(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("p0_aws_cloudwatch_logs.test", "cloudtrail"),
					resource.TestCheckResourceAttr("p0_aws_cloudwatch_logs.test", "state", "installed"),
					func(*terraform.State) error {
						item := f.item("aws-cloudwatch", "source", d.id)
						cloudtrail, _ := item["cloudtrail"].(map[string]any)
						if cloudtrail["type"] != "none" {
							return fmt.Errorf("P0 has the log source %v, want cloudtrail none", item)
						}
						return nil
					},
				),
			},
		},
	})
}

// P0 fixes the trail when it stages a log source, and generates the
// connector's read policy for its log group, so a new trail replaces both
// resources, and the new staging generates a policy for the new log group.
func TestAwsCloudWatchLogsCloudtrailReplaces(t *testing.T) {
	f := newLogSourceFake(t)
	d := logSourceTestDefaults
	changed := d
	changed.logGroupName = "org-trail"
	changed.coverage = "account"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLogSourcesDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.aws()},
			{
				Config: f.providerConfig() + changed.aws(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_aws_cloudwatch_logs_staged.test", plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction("p0_aws_cloudwatch_logs.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_aws_cloudwatch_logs.test", "cloudtrail.coverage", "account"),
					resource.TestCheckResourceAttr("p0_aws_cloudwatch_logs.test", "state", "installed"),
					resource.TestCheckResourceAttr("p0_aws_cloudwatch_logs_staged.test", "execution_policy", `{"Version":"2012-10-17","Statement":[{"Resource":"org-trail"}]}`),
				),
			},
		},
	})
}

// A failed install check fails the apply with P0's message, and leaves the
// log source staged. The next apply finishes the install.
func TestLogSourceInstallCheckFails(t *testing.T) {
	f := newLogSourceFake(t)
	d := logSourceTestDefaults
	var fail atomic.Bool
	fail.Store(true)
	f.failCheck = func(_ fakeItemKey, nextState string) bool { return fail.Load() && nextState == "configure" }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLogSourcesDestroyed(f.client()),
		Steps: []resource.TestStep{
			{
				Config:      f.providerConfig() + d.project(),
				ExpectError: regexp.MustCompile(`(?s)Google Cloud Logging install check failed.*The install check failed`),
			},
			{
				PreConfig: func() {
					fail.Store(false)
					if item := f.item("gcp-cloud-logging", "project", d.projectId); item["state"] != "stage" {
						t.Errorf("after the failed check, P0 has the project %v, want it staged", item)
					}
				},
				Config: f.providerConfig() + d.project(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_gcp_cloud_logging.test", "state", "installed"),
					resource.TestCheckResourceAttr("p0_gcp_cloud_logging.test", "connector_service_uri", logSourceServiceUri(fakeItemKey{"gcp-cloud-logging", "project", d.projectId})),
				),
			},
		},
	})
}

// A log source imported while P0 has it staged plans an update, and the apply
// takes the connector's URL that P0 finds when it installs.
func TestGcpCloudLoggingFinishedFromStage(t *testing.T) {
	f := newLogSourceFake(t)
	d := logSourceTestDefaults

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLogSourcesDestroyed(f.client()),
		Steps: []resource.TestStep{
			{Config: f.providerConfig() + d.organizationStaged()},
			{
				Config:                               f.providerConfig() + d.organization(),
				ResourceName:                         "p0_gcp_organization_cloud_logging.test",
				ImportState:                          true,
				ImportStateId:                        d.hostProjectId,
				ImportStatePersist:                   true,
				ImportStateVerifyIdentifierAttribute: "host_project_id",
			},
			{
				Config: f.providerConfig() + d.organization(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_gcp_organization_cloud_logging.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_gcp_organization_cloud_logging.test", "connector_service_uri", logSourceServiceUri(fakeItemKey{"gcp-cloud-logging", "organization", "_"})),
					resource.TestCheckResourceAttr("p0_gcp_organization_cloud_logging.test", "state", "installed"),
				),
			},
		},
	})
}

// The provider refuses at plan time what P0 would refuse.
func TestLogSourceInvalid(t *testing.T) {
	cases := []struct {
		name   string
		config func(logSourceTestInstall) string
		change func(*logSourceTestInstall)
		want   string
	}{
		{name: "AccountId", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.accountId = "12345" }, want: `12 numeric digits`},
		{name: "Region", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.region = "uswest2" }, want: `Invalid AWS region`},
		{name: "GovCloud", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.region = "us-gov-west-1" }, want: `Unsupported AWS region`},
		{name: "Coverage", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.coverage = "global" }, want: `value must be one of`},
		{name: "LogGroupName", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.logGroupName = "my trail" }, want: `Enter the CloudWatch log group`},
		{name: "Id", config: logSourceTestInstall.aws, change: func(d *logSourceTestInstall) { d.id = " prod" }, want: `Use a name of 1 to 255 characters`},
		{name: "ProjectId", config: logSourceTestInstall.project, change: func(d *logSourceTestInstall) { d.projectId = "My_Project" }, want: `Enter a Google Cloud project ID`},
		{name: "HostProjectId", config: logSourceTestInstall.organization, change: func(d *logSourceTestInstall) { d.hostProjectId = "p0" }, want: `Enter a Google Cloud project ID`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newLogSourceFake(t)
			d := logSourceTestDefaults
			c.change(&d)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      f.providerConfig() + c.config(d),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(c.want),
					},
				},
			})
		})
	}
}
