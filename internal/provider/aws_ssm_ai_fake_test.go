// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/p0-security/terraform-provider-p0/internal"
)

// A copy of redshift_ai_fake_test.go, for AWS SSM (AI), with the refusals of the
// app's install hooks on the shared fake's refuse hook, as log_source_fake_test.go
// has them.

// The AWS SSM (AI) connector's install schema, as the app defines it in
// packages/integrations/aws-ssm-ai/shared/src/components.ts: its fields, and those
// marked `step: "new"`, which P0 refuses to change after staging.
var awsSsmAiSchema = struct{ fields, stepNew []string }{
	fields:  []string{"region", "instanceRegions", "vault", "isSudoEnabled"},
	stepNew: []string{"region", "vault"},
}

// The app's `REGION` and `OTHER_PARTITION_REGION` in
// packages/integrations/aws-ssm-ai/shared/src/components.ts. The app also checks
// each region against its list of AWS regions, which the fake leaves out, so a
// mistyped region passes it.
var (
	awsSsmAiRegionRegex         = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d{1,2}$`)
	awsSsmAiOtherPartitionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)
)

func awsSsmAiIsCommercialRegion(region string) bool {
	return awsSsmAiRegionRegex.MatchString(region) && !awsSsmAiOtherPartitionRegex.MatchString(region)
}

// awsSsmAiFake fakes P0's install API for the AWS SSM (AI) connector, which P0 stores
// and checks as install-api.ts and configure.ts in the app do. It refuses what
// the connector's install hooks refuse without calling AWS: on staging, a
// missing or non-commercial region, and a vault outside the connector's account
// or region, and on configuring, a missing or malformed list of instance
// regions. It can't tell whether the AWS side of an install exists, so a test
// fails that check with failCheck.
type awsSsmAiFake struct {
	*fakeP0
	// Whether the step that moves the item to nextState fails its install check,
	// as when the connector can't reach SSM. P0 then saves nothing.
	failCheck func(key fakeItemKey, nextState string) bool
}

func newAwsSsmAiFake(t *testing.T) *awsSsmAiFake {
	f := &awsSsmAiFake{
		fakeP0:    newFakeP0(t),
		failCheck: func(fakeItemKey, string) bool { return false },
	}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "aws-ssm-ai" || key.component != "connector" {
			t.Errorf("unexpected item %v in the fake AWS SSM (AI) install", key)
			return
		}
		for field, value := range item {
			switch {
			case field == "state" || field == "label":
			case !slices.Contains(awsSsmAiSchema.fields, field):
				delete(item, field)
			default:
				if text, ok := value.(string); ok {
					item[field] = strings.TrimSpace(text)
				}
			}
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		updated := deepMerge(step.previous, step.body)
		if step.previous != nil && step.nextState != "stage" {
			for _, field := range awsSsmAiSchema.stepNew {
				if !reflect.DeepEqual(updated[field], step.previous[field]) {
					return http.StatusUnprocessableEntity, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
				}
			}
		}
		if message := awsSsmAiRefusal(step.key.id, step.nextState, updated); message != "" {
			return http.StatusBadRequest, message
		}
		if f.failCheck(step.key, step.nextState) {
			return http.StatusUnprocessableEntity, "The install check failed"
		}
		return 0, ""
	}
	return f
}

// What the app's assembler, verifier or configurer refuses in the item, in its
// words, or "" for an item they accept.
func awsSsmAiRefusal(awsAccountId, nextState string, item map[string]any) string {
	region, _ := item["region"].(string)
	vault, _ := item["vault"].(map[string]any)
	install, _ := vault["install"].(string)
	secretsRegion, _ := vault["secretsRegion"].(string)
	instanceRegions, _ := item["instanceRegions"].(string)
	switch {
	case strings.TrimSpace(region) == "":
		return "Choose the AWS region the connector runs in."
	case !awsSsmAiIsCommercialRegion(strings.TrimSpace(region)):
		return "Choose a commercial AWS region, such as us-west-2. P0 can't reach GovCloud or China."
	case strings.TrimSpace(install) != awsAccountId:
		return "Choose the connector's own AWS account as the AWS Secrets Manager installation, since the connector and the agents' roles reach secrets only there."
	case !awsSsmAiIsCommercialRegion(strings.TrimSpace(secretsRegion)):
		return "Enter a commercial AWS region for the secrets, such as us-west-2. P0 can't reach GovCloud or China."
	case nextState == "installed" && strings.TrimSpace(instanceRegions) == "":
		return "Enter the AWS regions whose instances agents can request, such as us-west-2,us-east-1."
	case nextState != "stage" && strings.TrimSpace(instanceRegions) != "":
		for _, instanceRegion := range strings.Split(instanceRegions, ",") {
			if !awsSsmAiIsCommercialRegion(strings.TrimSpace(instanceRegion)) {
				return fmt.Sprintf("%s isn't a commercial AWS region.", strings.TrimSpace(instanceRegion))
			}
		}
	}
	return ""
}

// Configures the provider to call the fake.
func (f *awsSsmAiFake) providerConfig() string {
	return providerConfig(f.fakeP0)
}

// A client of the fake's API, for checks that read P0's items directly, as tests
// against a real organization read them.
func (f *awsSsmAiFake) client() *internal.P0ProviderData {
	return &internal.P0ProviderData{BaseUrl: awsSsmAiOrgUrl(f.server.URL, fakeOrg), Client: f.server.Client()}
}

// The base URL of an organization's API, as the provider builds it.
func awsSsmAiOrgUrl(host string, org string) string {
	return fmt.Sprintf("%s/o/%s", host, org)
}
