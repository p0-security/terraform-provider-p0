package installtailscale

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

// copy of internal/provider/resources/install/auth0/common.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686), with the rules for a tailnet's ID.

const TailscaleKey = "tailscale"

// The tailnet component's key in P0's install API, beside installresources.Connector.
// It stays in this package, so adding Tailscale changes no shared code.
const tailnetComponent = "tailnet"

// P0's rules for the items' IDs, from packages/integrations/tailscale/shared/src/
// (components.ts and ids.ts) in the app.

// A connector's ID names its Terraform resources, and each tailnet picks its connector
// by it, so it follows a DNS label's rule: lowercase letters, digits and hyphens,
// starting and ending with a letter or digit, at most 63 characters
// (CONNECTOR_ID_PATTERN).
var ConnectorIdRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const ConnectorIdMaxLength = 63

var (
	// A Tailnet ID, as Tailscale's API names a tailnet in a path: the ID from Settings >
	// General, such as T1234CNTRL, `-` for the credential's own tailnet, or the legacy ID
	// that a tailnet created before October 2025 still answers to, such as example.com
	// (TAILNET_ID_PATTERN).
	tailnetIdPattern = regexp.MustCompile(`^(?:-|[A-Za-z0-9][A-Za-z0-9._@+-]{0,252})$`)
	// A tailnet's DNS name, such as tail1234.ts.net, which Tailscale's API doesn't take
	// in place of its ID.
	tailnetDnsNamePattern = regexp.MustCompile(`(?i)\.ts\.net$`)
)

// P0's message for a tailnet ID it refuses (INVALID_TAILNET_ID).
const invalidTailnetId = "Enter the Tailnet ID from Tailscale's Settings, General, such as T1234CNTRL. " +
	"The tailnet's DNS name, such as tail1234.ts.net, can't be used: Tailscale's API doesn't take it."

// Whether id is a Tailnet ID the connector takes (isTailnetId). None has a `/`, so an
// ID can go into a URL path as it is.
func isTailnetId(id string) bool {
	return tailnetIdPattern.MatchString(id) && !tailnetDnsNamePattern.MatchString(id)
}

// P0's rules for where the connector runs and where an OAuth client's secret is, and its
// messages for a value that breaks one, from
// packages/integrations/tailscale/backend/src/install/validation.ts in the app.

// P0's message for a region outside AWS's commercial partition (UNSUPPORTED_AWS_PARTITION).
const unsupportedAwsPartition = "P0's Tailscale integration supports only AWS's commercial regions, not GovCloud, China or other AWS partitions."

var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the OAuth client's secret", where: "the OAuth client's secret is in"}
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

// Used both for a connector's own ID and for the connector a tailnet names. The message
// is P0's (INVALID_CONNECTOR_ID) without its full stop, because the framework's
// validator ends the sentence with the value.
func connectorIdValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(ConnectorIdMaxLength),
		stringvalidator.RegexMatches(ConnectorIdRegex, fmt.Sprintf(
			"Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to %d characters",
			ConnectorIdMaxLength,
		)),
	}
}

// Refuses a value with P0's message alone, checking it as it is, as P0 checks an item's
// ID. The framework's stringvalidator.RegexMatches would add the value after P0's
// message, which already ends in a full stop.
type messageValidator struct {
	summary string
	message string
	test    func(value string) bool
}

var _ validator.String = messageValidator{}

func (v messageValidator) Description(context.Context) string {
	return v.message
}

func (v messageValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v messageValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !v.test(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, v.summary, v.message)
	}
}

// Refuses a tailnet's ID that isn't a Tailnet ID, wherever a tailnet is named, with
// P0's message (`validateTailnetId` in
// packages/integrations/tailscale/backend/src/install/validation.ts in the app).
func tailnetIdValidator() validator.String {
	return messageValidator{summary: "Invalid Tailnet ID", message: invalidTailnetId, test: isTailnetId}
}

// P0 runs the install checks through the connector, so a failed check usually points
// at the customer's setup, and P0's message says what's wrong. The summary and the
// detail don't guess at the cause, so they can't contradict P0's message.
func describeCheckError(kind string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("Tailscale %s install check failed", kind),
			fmt.Sprintf("P0 rejected the install check for the Tailscale %s %q:\n\n%s", kind, id, err)
	}
}

// Plans an update for an item that P0 hasn't installed, even when the configuration
// hasn't changed, so that the next apply finishes the install. An item gets here when
// someone imports it before its checks pass, or stages it again outside this resource.
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
