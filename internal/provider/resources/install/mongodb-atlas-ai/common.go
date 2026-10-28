package installmongodbatlasai

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

const MongodbAtlasAiKey = "mongodb-atlas-ai"

// All installable MongoDB Atlas components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "MongoDB Atlas"
	secretLabel      = "the Atlas service account's client secret"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

var atlasProjectIdRegex = regexp.MustCompile(`^[0-9a-f]{24}$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}

// P0 rejects a change to the project ID after staging, so a change replaces the
// install.
//
// Adds suffix to the description.
func atlasProjectIdAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The 24-character ID of the Atlas project, from Project Settings in Atlas." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(atlasProjectIdRegex, "Use the 24 lowercase hexadecimal characters of the project ID"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
