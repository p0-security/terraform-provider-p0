package installgitlabai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const GitlabAiKey = "gitlab-ai"

// All installable GitLab components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "GitLab"
	secretLabel      = "the GitLab group's admin token"
)

// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name.
var itemIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use only letters, digits and hyphens"),
	)
}
