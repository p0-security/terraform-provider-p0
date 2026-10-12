package installpagerdutyai

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

const PagerdutyAiKey = "pagerduty-ai"

// All installable PagerDuty components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "PagerDuty"
	secretLabel      = "the PagerDuty app's client secret"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// The subdomain goes into a space-separated scope list.
var subdomainRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

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
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}

// P0 rejects a change to the region after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func regionAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The PagerDuty service region that hosts the account. One of `us` (pagerduty.com) or `eu` (eu.pagerduty.com)." + suffix,
		Validators: []validator.String{
			stringvalidator.OneOf(regions...),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// P0 rejects a change to the subdomain after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func subdomainAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The subdomain of the PagerDuty account, as shown in its URL (e.g. `acme` in acme.pagerduty.com or acme.eu.pagerduty.com)." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(subdomainRegex, "Use only lowercase letters, digits and hyphens"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
