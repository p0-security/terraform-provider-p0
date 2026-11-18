// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// Each Access Logs log-source component's install schema, as the app defines
// it in packages/integrations/aws-cloudwatch/shared/src/components.ts and
// packages/integrations/gcp-cloud-logging/shared/src/components.ts: its fields,
// and those marked `step: "new"`, which P0 refuses to change after staging. The
// hidden fields that only P0 sets, such as the service account and warnings,
// are left out.
var logSourceSchema = map[string]map[string]struct{ fields, stepNew []string }{
	"aws-cloudwatch": {
		"source": {
			fields:  []string{"accountId", "region", "cloudtrail", "connectorName", "invokeRoleName", "invokeTrustPolicy", "executionPolicy"},
			stepNew: []string{"accountId", "region", "cloudtrail"},
		},
	},
	"gcp-cloud-logging": {
		"project": {
			fields:  []string{"hostProjectId", "connectorServiceName", "connectorServiceAccount", "region", "connectorServiceUri"},
			stepNew: []string{"hostProjectId"},
		},
		"organization": {
			fields:  []string{"hostProjectId", "connectorServiceName", "connectorServiceAccount", "region", "connectorServiceUri"},
			stepNew: []string{"hostProjectId"},
		},
	},
}

// logSourceFake fakes P0's install API for the log-source items, which P0
// stores and checks as install-api.ts and configure.ts in the app do, and
// generates fields as the integrations' installers do: the connector's names
// and policies when an item is staged, and a GCP connector's URL when it is
// verified. It runs no install checks; a test fails one with failCheck.
type logSourceFake struct {
	*fakeP0
	// Whether the step that moves the item to nextState fails its install check.
	failCheck func(key fakeItemKey, nextState string) bool
}

func newLogSourceFake(t *testing.T) *logSourceFake {
	f := &logSourceFake{
		fakeP0:    newFakeP0(t),
		failCheck: func(fakeItemKey, string) bool { return false },
	}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		schema, ok := logSourceSchema[key.integration][key.component]
		if !ok {
			t.Errorf("unexpected item %v in the fake log-source install", key)
			return
		}
		for field := range item {
			if field != "state" && field != "label" && !slices.Contains(schema.fields, field) {
				delete(item, field)
			}
		}
		// The installers keep a generated field's saved value.
		for field, value := range logSourceGenerated(key, item) {
			if _, ok := item[field]; !ok {
				item[field] = value
			}
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.nextState == "stage" || step.previous == nil {
			return 0, ""
		}
		updated := deepMerge(step.previous, step.body)
		for _, field := range logSourceSchema[step.key.integration][step.key.component].stepNew {
			if !reflect.DeepEqual(updated[field], step.previous[field]) {
				return http.StatusUnprocessableEntity, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
			}
		}
		if f.failCheck(step.key, step.nextState) {
			return http.StatusUnprocessableEntity, "The install check failed"
		}
		return 0, ""
	}
	return f
}

// The fields that P0 generates for an item at its state.
func logSourceGenerated(key fakeItemKey, item map[string]any) map[string]any {
	switch key.integration {
	case "aws-cloudwatch":
		region, _ := item["region"].(string)
		return map[string]any{
			"connectorName":     "p0-access-logs",
			"invokeRoleName":    "P0AccessLogsInvoker-" + region,
			"invokeTrustPolicy": `{"Version":"2012-10-17","Statement":[]}`,
			"executionPolicy":   logSourceExecutionPolicy(item),
		}
	case "gcp-cloud-logging":
		name := key.id
		if key.component == "organization" {
			name = "org"
		}
		host, _ := item["hostProjectId"].(string)
		generated := map[string]any{
			"connectorServiceName":    "p0-logs-" + name,
			"connectorServiceAccount": fmt.Sprintf("p0-logs-%s@%s.iam.gserviceaccount.com", name, host),
			"region":                  "us-central1",
		}
		if item["state"] != "stage" {
			generated["connectorServiceUri"] = logSourceServiceUri(key)
		}
		return generated
	}
	return nil
}

// Names the CloudTrail log group, if any, so that a test can tell which trail
// the policy was generated for.
func logSourceExecutionPolicy(item map[string]any) string {
	group := "none"
	if cloudtrail, ok := item["cloudtrail"].(map[string]any); ok {
		if name, ok := cloudtrail["logGroupName"].(string); ok {
			group = name
		}
	}
	return fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Resource":"%s"}]}`, group)
}

func logSourceServiceUri(key fakeItemKey) string {
	return fmt.Sprintf("https://%s-%s.a.run.app", key.component, strings.ReplaceAll(key.id, "_", "org"))
}

func (f *logSourceFake) providerConfig() string {
	return providerConfig(f.fakeP0)
}

func (f *logSourceFake) client() *internal.P0ProviderData {
	return &internal.P0ProviderData{BaseUrl: fmt.Sprintf("%s/o/%s", f.server.URL, fakeOrg), Client: f.server.Client()}
}

// The item behind each log-source resource type.
var logSourceItemByResourceType = map[string]struct{ integration, component, idAttribute string }{
	"p0_aws_cloudwatch_logs_staged":            {"aws-cloudwatch", "source", "id"},
	"p0_aws_cloudwatch_logs":                   {"aws-cloudwatch", "source", "id"},
	"p0_gcp_cloud_logging_staged":              {"gcp-cloud-logging", "project", "project_id"},
	"p0_gcp_cloud_logging":                     {"gcp-cloud-logging", "project", "project_id"},
	"p0_gcp_organization_cloud_logging_staged": {"gcp-cloud-logging", "organization", ""},
	"p0_gcp_organization_cloud_logging":        {"gcp-cloud-logging", "organization", ""},
}

// checkLogSourcesDestroyed checks that P0 no longer has the item behind any
// log-source resource in the destroyed state.
func checkLogSourcesDestroyed(client *internal.P0ProviderData) func(*terraform.State) error {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			item, ok := logSourceItemByResourceType[rs.Type]
			if !ok {
				continue
			}
			id := "_"
			if item.idAttribute != "" {
				id = rs.Primary.Attributes[item.idAttribute]
			}
			path := fmt.Sprintf("integrations/%s/config/%s/%s", item.integration, item.component, url.PathEscape(id))
			resp, err := client.Get(path, &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the %s %s was deleted: %w", rs.Type, id, err)
			}
			return fmt.Errorf("the %s %s is still installed in P0", rs.Type, id)
		}
		return nil
	}
}
