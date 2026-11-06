package installentraid

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

const (
	EntraIdKey = "entra-id"
	// P0 makes this component's item itself when an iam-write item is staged, under the
	// same id. It holds the service account that P0 signs in to Entra ID as.
	baseComponent = "base"
)

var sovereignClouds = []string{"AzureCloud", "AzureUSGovernment"}

var emailFields = []string{"userPrincipalName", "mail", "otherMails"}

// The Entra tenant ID is the item's id, so a change replaces the install.
//
// Adds suffix to the description.
func tenantIdAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The Microsoft Entra ID tenant (directory) ID, a UUID." + suffix,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
		Validators: []validator.String{
			stringvalidator.RegexMatches(common.UuidRegex, "Entra tenant ID must be a valid UUID"),
		},
	}
}

// P0 fixes the cloud at staging, so a change replaces the install.
//
// Adds suffix to the description.
func sovereignCloudIdAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(sovereignClouds[0]),
		MarkdownDescription: "The Azure cloud the tenant is in: `AzureCloud` (portal.azure.com, the default) or `AzureUSGovernment` (portal.azure.us). P0 fixes it at staging, so a change replaces the install." + suffix,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
		Validators: []validator.String{
			stringvalidator.OneOf(sovereignClouds...),
		},
	}
}

// P0 holds the cloud and the email field as objects with the chosen option in "type".
type optionJson struct {
	Type string `json:"type"`
}

func optionToJson(value types.String) *optionJson {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return &optionJson{Type: value.ValueString()}
}

// P0 defaults both options, so an item without one, such as an import of an install
// made in the app, matches the default and isn't replaced.
func optionFromJson(value *optionJson, fallback string) types.String {
	if value == nil {
		return types.StringValue(fallback)
	}
	return types.StringValue(value.Type)
}

func basePath(tenantId string) string {
	return fmt.Sprintf("integrations/%s/config/%s/%s", EntraIdKey, baseComponent, tenantId)
}

// On configure, P0 signs in to the tenant as the caller app and lists a group and a
// role. A failure usually means the app, its federated credential or its Graph
// permissions aren't set up yet, and P0's message says which.
func describeCheckError(id string, err error) (string, string) {
	return "Entra ID install check failed",
		fmt.Sprintf("P0 could not read the Entra tenant %q as the caller app:\n\n%s", id, err)
}
