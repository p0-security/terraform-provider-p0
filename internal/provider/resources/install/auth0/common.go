package installauth0

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

// copy of internal/provider/resources/install/docker/common.go, renamed for Auth0
// (INT-590).

const Auth0Key = "auth0"

// P0's rules for the two items' IDs, from
// packages/integrations/auth0/shared/src/components.ts in the app.

// A connector's ID names its Terraform resources, and each tenant picks its connector
// by it, so it follows a DNS label's rule: lowercase letters, digits and hyphens,
// starting and ending with a letter or digit, at most 63 characters
// (CONNECTOR_ID_PATTERN).
var ConnectorIdRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const ConnectorIdMaxLength = 63

// An Auth0 tenant's canonical domain, `<name>[.<region>].auth0.com`, which is the
// tenant's item ID (AUTH0_TENANT_PATTERN). The connector refuses any other domain,
// because the Management API's audience is always the canonical domain.
var TenantRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?(?:\.[a-z]{2})?\.auth0\.com$`)

// P0's message for a tenant ID that isn't a canonical domain (INVALID_TENANT_DOMAIN).
const invalidTenantDomain = "Enter the tenant's canonical Auth0 domain, such as acme.us.auth0.com, in lowercase. " +
	"A custom domain can't be used: the Auth0 Management API only answers on the canonical one."

// Auth0's rule for a client ID: letters, digits, `-` and `_`, 1 to 128 characters
// (AUTH0_CLIENT_ID_PATTERN).
var ClientIdRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// P0's message for a client ID it refuses (INVALID_CLIENT_ID). It leaves the value out,
// because a customer may have pasted the client secret in its place.
const invalidClientId = "Enter the client ID of the Auth0 machine-to-machine application, as the application's settings show it."

// P0's rules for where the connector runs and where the client secret is, and its
// messages for a value that breaks one, from
// packages/integrations/auth0/backend/src/install/validation.ts in the app.

// P0's message for a region outside AWS's commercial partition.
const unsupportedAwsPartition = "P0's Auth0 integration supports only AWS's commercial regions. Enter a commercial region, such as us-west-2."

var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the client secret", where: "the client secret is in"}
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

// Used both for a connector's own ID and for the connector a tenant names. The message
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

// P0 runs the install checks through the connector, so a failed check usually points
// at the customer's setup, and P0's message says what's wrong. The summary and the
// detail don't guess at the cause, so they can't contradict P0's message.
func describeCheckError(kind string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("Auth0 %s install check failed", kind),
			fmt.Sprintf("P0 rejected the install check for the Auth0 %s %q:\n\n%s", kind, id, err)
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
