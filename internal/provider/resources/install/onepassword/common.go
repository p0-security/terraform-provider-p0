package installonepassword

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

const OnePasswordKey = "1password"

// P0's rules for the two items' IDs
// (packages/integrations/one-password/shared/src/components.ts in the app).

// A connector's ID names its Terraform resources, and each vault picks its
// connector by it: lowercase letters, digits and hyphens, starting and ending with a
// letter or digit, at most 63 characters (CONNECTOR_ID_PATTERN).
var ConnectorIdRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const ConnectorIdMaxLength = 63

// 1Password's form for a vault's ID: 26 lowercase letters and digits, the base32 form
// 1Password gives every vault (ONEPASSWORD_VAULT_ID_PATTERN). The connector refuses any
// other ID.
var VaultIdRegex = regexp.MustCompile(`^[a-z0-9]{26}$`)

// P0's rules for where the connector runs and where the token's secret is, and its
// messages for a value that breaks one
// (packages/integrations/one-password/backend/src/install/validation.ts in the app).

// P0's message for a region outside AWS's commercial partition.
const unsupportedAwsPartition = "P0's 1Password integration supports only AWS's commercial regions for now."

// What a hosting or secret manager field places. P0's messages say where it is.
var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the service account token's secret", where: "the token's secret is in"}
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

// The checks on a connector's ID, where it is an item's own ID and where an
// vault names its connector by it. The message is P0's (INVALID_CONNECTOR_ID),
// without its full stop: the framework's validator ends the sentence with the value.
func connectorIdValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(ConnectorIdMaxLength),
		stringvalidator.RegexMatches(ConnectorIdRegex, fmt.Sprintf(
			"Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to %d characters",
			ConnectorIdMaxLength,
		)),
	}
}

// P0 runs the install checks through the connector, so a failed check usually means a
// problem in the customer's setup, which P0's message describes. The wording doesn't
// presume where the problem is, and leaves that to P0's message.
func describeCheckError(kind string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("1Password %s install check failed", kind),
			fmt.Sprintf("P0 rejected the install check for the 1Password %s %q:\n\n%s", kind, id, err)
	}
}

// Plans an update for an item that P0 hasn't installed, even when the configuration
// hasn't changed, so that the next apply finishes the install. That covers an item
// imported before its checks passed, and one that was staged again outside this
// resource. P0 refuses a second PUT of an item that exists, so staging it again from
// here isn't a way to finish. An installed item keeps the framework's plan, which shows
// no difference while the configuration matches it.
//
// As in any update the framework plans, the attributes that only P0 sets are unknown
// until apply: the item's state, and those at computed, which the install checks
// assign. Update takes them from P0's response.
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
