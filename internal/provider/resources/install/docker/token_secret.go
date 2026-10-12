package installdocker

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Where an organization's access token is, in the customer's own secret manager: P0's
// rules for the vault's fields and for the secret's name, as it checks them when it
// creates the item (`validateTokenSecret` in
// packages/integrations/docker/backend/src/install/validation.ts in the app). P0 gives
// the connector a pointer to the secret on every call
// (packages/integrations/docker/shared/src/token-secret.ts): on AWS the partial ARN
// arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>, and on Google
// Cloud the secret's project and name. P0 never holds the token.

var (
	// Google's rule for a secret's ID, which admits no `/`, so a full resource name
	// doesn't match.
	gcpSecretIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
	// How every Docker access token starts: `dckr_`, then its kind, such as `dckr_oat_`
	// for an organization access token. The rest is letters, digits, hyphens and
	// underscores, which either secret manager's rule for a name would take.
	dockerTokenPattern = regexp.MustCompile(`^dckr_`)
)

// P0's messages for an access token secret name it refuses. None repeats the value,
// which may be a token pasted in place of the name.
const (
	invalidSecretName  = "That doesn't look like a secret name. Enter the name of the secret that holds the organization access token, not its ARN, its full resource name or the token itself."
	pastedToken        = "That's a Docker access token, not the name of its secret. Enter the name of the secret that holds the token. P0 never needs the token itself, so generate a new one if it was shared anywhere else."
	missingSecretName  = "Enter the name of the secret that holds, or will hold, the organization access token."
	suffixedSecretName = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
)

// The rules for each vault type's fields, in the order P0 checks them.
var vaultRules = map[string][]fieldRules{
	awsSecretsManager: {
		{name: "account_id", rules: awsAccountRules(secretPlace)},
		{name: "secrets_region", rules: awsRegionRules(secretPlace)},
	},
	gcpSecretManager: {
		{name: "project_id", rules: gcpProjectRules(secretPlace)},
	},
}

// Refuses a Docker access token in place of its secret's name. It holds for a secret
// in any vault.
var notAToken = rule{
	summary: "Docker access token in place of its secret's name",
	message: pastedToken,
	test:    func(name string) bool { return !dockerTokenPattern.MatchString(name) },
}

// The rules for the name of a secret in each vault, in the order P0 checks them. A
// pasted token is refused first, since each vault's own rule would take one. The next
// says that it's no name at all.
var tokenSecretNameRules = map[string][]rule{
	awsSecretsManager: {
		notAToken,
		{
			summary: "Invalid access token secret name",
			message: invalidSecretName,
			test:    awsSecretNamePattern.MatchString,
		},
		{
			summary: "Access token secret name ends like an ARN suffix",
			message: suffixedSecretName,
			test: func(name string) bool {
				return !awsSecretNameSuffixPattern.MatchString(name)
			},
		},
	},
	gcpSecretManager: {
		notAToken,
		{
			summary: "Invalid access token secret name",
			message: invalidSecretName,
			test:    gcpSecretIdPattern.MatchString,
		},
	},
}

// Rejects a vault block whose fields don't match its type, each vault field that P0
// refuses for where the secret is, and a secret name that P0 refuses for that vault. A
// vault of a type that isn't known yet is skipped, and so is the name's form, which
// depends on the vault. A pasted token and whitespace around the name don't, so they
// are refused even then: a token must never show in a plan or reach P0. P0 also refuses
// a vault outside the connector's cloud, which this can't check: it knows the connector
// only by its ID.
func validateTokenSecret(vault *vaultModel, name types.String, diags *diag.Diagnostics) {
	validateVault(vault, diags)
	if vault != nil && isSet(vault.Type) {
		validateFields(path.Root("vault"), vault.fields(), vaultRules[vault.Type.ValueString()], diags)
	}

	if !isSet(name) {
		return
	}
	// P0 checks that the name is there before it trims it.
	if name.ValueString() == "" {
		diags.AddAttributeError(path.Root("token_secret_name"), "Missing access token secret name", missingSecretName)
		return
	}
	// Until the vault's type is known, only the rule that holds for any vault applies.
	rules := []rule{notAToken}
	if vault != nil {
		if ofVault, ok := tokenSecretNameRules[vault.Type.ValueString()]; ok {
			rules = ofVault
		}
	}
	validateField(path.Root("token_secret_name"), name, rules, diags)
}
