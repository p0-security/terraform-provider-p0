package installservicenowai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const ServiceNowAiKey = "servicenow-ai"

const instanceComponent = "instance"

const (
	integrationLabel = "ServiceNow"
	secretLabel      = "the connector's client secrets and signing key"
)

// The id is the first label of the instance host, and part of the GCP secret's
// name.
var instanceIdRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(instanceIdRegex, "Use only lowercase letters, digits and hyphens"),
	)
}
