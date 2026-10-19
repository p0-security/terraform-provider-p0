package installentraagentidentity

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const EntraAgentIdentityKey = "entra-agent-identity"

const previewNote = "**Note:** This integration is currently in preview."

const docsLink = "For the full install steps, see [Entra Agent Identity Creator](https://docs.p0.dev/integrations/resource-integrations/entra-agent-identity-creator)."

const supportEmail = "support@p0.dev"

// P0's rules for the install's fields, from the app's
// packages/integrations/entra-agent-identity/shared/src/components.ts.
var (
	// A directory, subscription or client ID. P0 accepts only the lowercase form,
	// which Entra and the Azure portal show.
	lowercaseGuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// An Azure region's name, e.g. eastus.
	azureRegionRegex = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	// Microsoft Graph app role values separated by commas. P0 trims the whole value
	// before it stores it, so whitespace is allowed only around the commas: a value
	// P0 stored differently would never match the plan.
	appRoleAllowlistRegex = regexp.MustCompile(`^[A-Za-z][\w.-]*(\s*,\s*[A-Za-z][\w.-]*)*$`)
)

func guidValidator(label string) validator.String {
	return stringvalidator.RegexMatches(lowercaseGuidRegex, fmt.Sprintf("Enter the %s as a lowercase GUID, e.g. 00000000-0000-0000-0000-000000000000", label))
}

// The item's ID. P0 keys the install by the directory, so a change replaces it.
func directoryIdAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          []validator.String{guidValidator("directory (tenant) ID")},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// The item component is at "dev" maturity in P0, which offers it only in development
// environments: everywhere else P0 answers the request that stages the item, and the
// verify and configure steps, with a bare 404. P0 still answers reads, so a missing
// item on read means it was removed, and the resource leaves state as usual.
func describeNotOffered(id string, err error) (string, string) {
	return "Entra Agent Identity Creator isn't available to this P0 organization",
		fmt.Sprintf("P0 couldn't stage or install the Entra Agent Identity Creator for directory %q. "+
			"This usually means P0 doesn't offer it to your organization yet: the integration is in preview, and P0 offers it only to development organizations until its release. "+
			"Contact %s with questions.\n\n"+
			"P0's response: %s", id, supportEmail, err)
}
