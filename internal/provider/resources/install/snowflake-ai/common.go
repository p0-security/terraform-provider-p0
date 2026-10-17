package installsnowflakeai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const SnowflakeAiKey = "snowflake-ai"

// All installable Snowflake components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "Snowflake"
	secretLabel      = "the Snowflake connector user's private key"
)

const betaNote = "**Note:** This integration is currently in beta."

// The id is a Snowflake account identifier in its `ORGNAME-ACCOUNTNAME` form.
// The connector's IAM condition matches the id up to the next `_` in each GCP
// secret name, so P0 writes each `_` of the id as `-` there. An id has
// exactly one `-`, between the organization and the account, so that mapping
// gives each id its own secret names.
//
// TODO(INT-612): placeholder, account identifier format not decided.
var itemIdRegex = regexp.MustCompile(`^[A-Z0-9]+-[A-Z0-9_]+$`)

const itemIdRule = "the account identifier in its ORGNAME-ACCOUNTNAME form, in uppercase letters, digits and underscores, with one hyphen between the organization and the account"

func idAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.RegexMatches(itemIdRegex, "Use "+itemIdRule),
	)
}
