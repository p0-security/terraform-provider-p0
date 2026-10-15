package installsalesforceai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const SalesforceAiKey = "salesforce-ai"

const orgComponent = "org"

const (
	integrationLabel = "Salesforce"
	secretLabel      = "the P0 Provisioner app's consumer secret"
)

// The id is part of the GCP secret's name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// P0 sends the consumer secret only to a Salesforce My Domain host.
var fullDomainRegex = regexp.MustCompile(`^[\w.-]+\.my\.salesforce\.com$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}

// P0 rejects a change to the full domain after staging, so a change replaces
// the install.
//
// Adds suffix to the description.
func fullDomainAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The org's full My Domain host, without `https://` or a trailing `/` (e.g. `acme.my.salesforce.com` or `acme.develop.my.salesforce.com`). Omit it when the host is `<id>.my.salesforce.com`." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(fullDomainRegex, "Enter only the host, ending with my.salesforce.com (no https:// or /)"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
