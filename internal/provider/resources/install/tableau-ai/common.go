package installtableauai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const TableauAiKey = "tableau-ai"

const (
	integrationLabel = "Tableau"
	secretLabel      = "the admin connected app"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// The connector accepts only the bare pod URL.
var serverRegex = regexp.MustCompile(`^https://[a-z0-9-]+\.online\.tableau\.com$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}

// P0 rejects a change to the server after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func serverAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The site's Tableau Cloud pod URL, as shown before `/#/` in its address, without a trailing `/` (e.g. `https://us-east-1.online.tableau.com`)." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(serverRegex, "Enter the pod URL as https://<pod>.online.tableau.com, with no trailing /"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// P0 rejects a change to the content URL after staging, so a change replaces
// the install.
//
// Adds suffix to the description.
func contentUrlAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The site name (its content URL), as shown after `/site/` in its address (e.g. `acme` in `https://us-east-1.online.tableau.com/#/site/acme/home`)." + suffix,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
