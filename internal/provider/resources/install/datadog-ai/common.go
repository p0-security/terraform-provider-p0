package installdatadogai

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

const DatadogAiKey = "datadog-ai"

// All installable Datadog components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "Datadog"
	secretLabel      = "the Datadog organization's admin keys"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

var sites = []string{"us1", "us3", "us5", "eu1", "ap1", "ap2", "us1-fed"}

type siteJson struct {
	Type string `json:"type"`
}

func siteToJson(site types.String) *siteJson {
	if site.IsNull() || site.IsUnknown() {
		return nil
	}
	return &siteJson{Type: site.ValueString()}
}

func siteFromJson(json *siteJson) types.String {
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

// P0 rejects a change to the site after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func siteAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The Datadog site that hosts the organization, as shown in its URL. One of `us1` (datadoghq.com), `us3`, `us5`, `eu1` (datadoghq.eu), `ap1`, `ap2` or `us1-fed` (ddog-gov.com)." + suffix,
		Validators: []validator.String{
			stringvalidator.OneOf(sites...),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
