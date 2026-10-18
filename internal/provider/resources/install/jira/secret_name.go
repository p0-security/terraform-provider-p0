package installjira

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// P0's rules for the name of the secret that holds the site's OAuth client secret,
// and its messages for a name it refuses (findSecretNameProblem in
// packages/integrations/jira/shared/src/secret-name.ts in the app). The name is a
// secret's name in the site's vault, never an ARN or a resource name: P0 builds the
// secret's address from the name and the vault.
var (
	// Neither pattern admits whitespace, a colon, a quote or a wildcard, which keeps
	// out an ARN and a wildcard.
	awsSecretNameRegex = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	gcpSecretNameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
	// The hyphen and six letters or digits that Secrets Manager adds to a secret's name
	// in its full ARN. It reads a partial ARN that ends this way as a full one, and
	// finds no secret by it.
	awsArnSuffixRegex = regexp.MustCompile(`-[A-Za-z0-9]{6}$`)
)

const (
	invalidSecretNameSummary = "Invalid client secret name"
	invalidSecretName        = "That doesn't look like a secret name. Enter the name of the secret that holds the Jira client secret, " +
		"not its ARN or the client secret itself."
	invalidGcpSecretName = "That isn't a secret's name in Google Secret Manager. Enter the name alone, without its project or a version: " +
		"letters, digits, underscores and hyphens, up to 255 characters."
	suffixedSecretName = "Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters, like -AbCdEf. " +
		"Use a secret whose name doesn't end that way, or one in the connector's own account."
)

// Rejects a client secret name that P0 refuses for the vault the configuration names.
// The vault's type picks the rule: one that isn't known yet gets the rule that a name
// in any vault meets. P0 points a connector on AWS at a secret in another account by
// its partial ARN, which Secrets Manager reads as a full one when the name ends like
// an ARN's suffix, so such a name is refused when both accounts are known and differ.
// No message repeats the name, which may be the client secret pasted in its place.
func validateSecretName(name types.String, vault *vaultModel, hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	rules := []rule{{summary: invalidSecretNameSummary, message: invalidSecretName, test: awsSecretNameRegex.MatchString}}
	if vault != nil && installapp.IsSet(vault.Type) && vault.Type.ValueString() == installvaultedcredential.GcpSecretManager {
		rules = []rule{{summary: invalidSecretNameSummary, message: invalidGcpSecretName, test: gcpSecretNameRegex.MatchString}}
	}
	if isCrossAccount(vault, hosting) {
		rules = append(rules, rule{
			summary: "Client secret name ends like an ARN suffix",
			message: suffixedSecretName,
			test:    func(name string) bool { return !awsArnSuffixRegex.MatchString(name) },
		})
	}
	validateField(path.Root("client_secret_name"), name, rules, diags)
}

// Whether the vault is AWS Secrets Manager in another account than the connector's,
// as far as the configuration says. P0 compares the two accounts trimmed.
func isCrossAccount(vault *vaultModel, hosting *installapp.ConnectorHostingModel) bool {
	if vault == nil || hosting == nil || !installapp.IsSet(vault.Type) || !installapp.IsSet(hosting.Type) {
		return false
	}
	if vault.Type.ValueString() != awsSecretsManager || hosting.Type.ValueString() != installapp.AwsHosting {
		return false
	}
	if !installapp.IsSet(vault.AccountId) || !installapp.IsSet(hosting.AccountId) {
		return false
	}
	return trimmed(vault.AccountId) != trimmed(hosting.AccountId)
}
