package installgitguardianai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const GitguardianAiKey = "gitguardian-ai"

// All installable GitGuardian components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "GitGuardian"
	secretLabel      = "the GitGuardian workspace's admin token"
)

// The item ID is the GitGuardian workspace ID, which is numeric. P0 refuses
// any other.
var itemIdRegex = regexp.MustCompile(`^\d+$`)

var regions = []string{"us", "eu"}

type regionJson struct {
	Type string `json:"type"`
}

func regionToJson(region types.String) *regionJson {
	if region.IsNull() || region.IsUnknown() {
		return nil
	}
	return &regionJson{Type: region.ValueString()}
}

func regionFromJson(json *regionJson) types.String {
	if json == nil {
		return types.StringNull()
	}
	return types.StringValue(json.Type)
}

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use the GitGuardian workspace ID (digits only)"),
	)
}

// P0 rejects a change to the region after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func regionAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The GitGuardian region that hosts the workspace. One of `us` (dashboard.gitguardian.com) or `eu` (dashboard.eu1.gitguardian.com)." + suffix,
		Validators: []validator.String{
			stringvalidator.OneOf(regions...),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
