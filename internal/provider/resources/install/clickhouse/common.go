package installclickhouse

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// copy of internal/provider/resources/install/auth0/common.go (#245),
// itself a copy of Docker Hub's, renamed for ClickHouse Cloud (INT-695).

const ClickHouseKey = "clickhouse"

// P0's rules for the two items' IDs, from
// packages/integrations/clickhouse/shared/src/components.ts in the app.

// A connector's ID names its Terraform resources, and each organization picks its
// connector by it, so it follows a DNS label's rule: lowercase letters, digits and
// hyphens, starting and ending with a letter or digit, at most 63 characters
// (CONNECTOR_ID_PATTERN).
var ConnectorIdRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const ConnectorIdMaxLength = 63

// A ClickHouse Cloud organization's ID, a UUID of version 1 to 8 and the standard
// variant, in lowercase, which is the organization's item ID
// (CLICKHOUSE_ORG_ID_PATTERN). P0 refuses an ID in capitals rather than rewriting
// it, so the item's key matches what the configuration names.
var OrganizationIdRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// P0's message for an organization ID that isn't a lowercase UUID
// (INVALID_ORGANIZATION_ID).
const invalidOrganizationId = "Enter the ClickHouse Cloud organization ID, a UUID such as 0b5a3c1e-6f2d-4e8a-9c7b-1d2e3f4a5b6c, in lowercase. " +
	"The ClickHouse Cloud console shows it in the organization's details."

// P0's rules for where the connector runs and where the API key's secret is, and its
// messages for a value that breaks one, from
// packages/integrations/clickhouse/backend/src/install/validation.ts in the app.

// P0's message for a region outside AWS's commercial partition
// (UNSUPPORTED_AWS_PARTITION).
const unsupportedAwsPartition = "P0's ClickHouse Cloud integration supports only AWS's commercial regions."

var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the API key's secret", where: "the API key's secret is in"}
)

// A Google Cloud project's ID: 6 to 30 lowercase letters, digits and hyphens, starting
// with a letter and ending with a letter or digit.
var gcpProjectPattern = regexp.MustCompile(`^[a-z][-a-z0-9]{4,28}[a-z0-9]$`)

func gcpProjectRules(p place) []rule {
	return []rule{{
		summary: "Invalid Google Cloud project for " + p.what,
		message: fmt.Sprintf("Enter the ID of the Google Cloud project %s.", p.where),
		test:    gcpProjectPattern.MatchString,
	}}
}

// Used both for a connector's own ID and for the connector an organization names. The
// message is P0's (INVALID_CONNECTOR_ID) without its full stop, because the
// framework's validator ends the sentence with the value.
func connectorIdValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(ConnectorIdMaxLength),
		stringvalidator.RegexMatches(ConnectorIdRegex, fmt.Sprintf(
			"Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to %d characters",
			ConnectorIdMaxLength,
		)),
	}
}

// P0 runs the install checks through the connector, so a failed check usually points
// at the customer's setup, and P0's message says what's wrong. The summary and the
// detail don't guess at the cause, so they can't contradict P0's message.
func describeCheckError(kind string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("ClickHouse Cloud %s install check failed", kind),
			fmt.Sprintf("P0 rejected the install check for the ClickHouse Cloud %s %q:\n\n%s", kind, id, err)
	}
}

// Plans an update for an item that P0 hasn't installed, even when the configuration
// hasn't changed, so that the next apply finishes the install. An item gets here when
// someone imports it before its install finishes, or stages it again outside this
// resource.
// P0 refuses a second PUT of an item that exists, so staging it again can't finish
// the install. An installed item keeps the framework's plan, which shows no difference
// while the configuration matches it.
//
// As in any update the framework plans, the attributes only P0 sets stay unknown
// until apply: the item's state, and the paths in computed, which the install checks
// assign. Update reads them from P0's response.
func planUnfinishedInstall(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, computed ...path.Path) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	for _, attribute := range append([]path.Path{path.Root("state")}, computed...) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, attribute, types.StringUnknown())...)
	}
}
