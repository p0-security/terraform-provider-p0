package installlookerai

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const LookerAiKey = "looker-ai"

// All installable Looker components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "Looker"
	secretLabel      = "the Looker instance's Admin key"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// `https://` and a lowercase name under looker.app, and nothing else. The P0 app
// also accepts uppercase and a trailing slash, but the connector accepts only this
// form, and sends the Admin key only to such a host. Holding the provider to it
// means a configured URL never differs from the one the connector is sent.
var instanceUrlRegex = regexp.MustCompile(`^https://[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*\.looker\.app$`)

// Google Secret Manager's rule for a secret's ID. It admits no `/`, so a secret's
// resource path doesn't match.
var secretIdRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// One path segment, as in a folder's URL: /folders/<id>, at most 64 characters as the
// connector allows.
var folderIdRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}

// A grant's Looker service account lives in this instance, so P0 rejects a change
// to the URL after staging, and a change replaces the install.
//
// Adds suffix to the description.
func instanceUrlAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The base URL of the Looker (Google Cloud core) instance, `https://<name>.looker.app`, in lowercase with no path or trailing slash. P0 reaches the instance over the internet, so it needs a public endpoint. Custom domains and ports aren't supported." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(instanceUrlRegex, "Enter the instance's base URL in lowercase, with no path or trailing slash, e.g. https://example.looker.app. Custom domains and ports aren't supported."),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// The connector's deployment grants it read on exactly this secret, so P0 rejects a
// change to it after staging, and a change replaces the install.
//
// Adds suffix to the description.
func adminKeySecretAttribute(suffix string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The ID of the Google Secret Manager secret, in `secret_manager.project_id`, that holds the Looker API key of a Looker service account with the Admin role. Only the connector reads it, never P0." + suffix,
		Validators: []validator.String{
			stringvalidator.RegexMatches(secretIdRegex, "Enter the secret's name, not its resource path or the key itself. Use only letters, digits, hyphens and underscores."),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// P0 runs the install check through the connector, which signs in to the instance
// with the Admin key, so a failed check usually means a problem in the customer's
// setup, which P0's message describes. The wording doesn't presume where the problem
// is, and leaves that to P0's message.
func describeCheckError(id string, err error) (string, string) {
	return "Looker install check failed",
		fmt.Sprintf("P0 rejected the install check for the Looker instance %q:\n\n%s", id, err)
}
