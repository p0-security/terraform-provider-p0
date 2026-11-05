package installcloudflareai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const CloudflareAiKey = "cloudflare-ai"

// All installable Cloudflare components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "Cloudflare"
	secretLabel      = "the Cloudflare account's install token"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name. Cloudflare caps token names at 120 characters, and a grant
// token's name holds the id, so the id is at most 64.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var accountIdRegex = regexp.MustCompile(`^[0-9a-f]{32}$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens, up to 64 characters"),
	)
}

// P0 rejects a change to the account ID after staging, so a change replaces
// the install.
//
// Adds suffix to the description.
func accountIdAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The 32-character ID of the Cloudflare account, from the account's home page in the Cloudflare dashboard." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(accountIdRegex, "Use the 32 lowercase hexadecimal characters of the account ID"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
