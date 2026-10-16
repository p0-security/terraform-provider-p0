package installonepassword

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Where an account's service account token is, in the customer's own secret manager: P0's
// rules for the secret manager's fields and for the secret's name, as it checks them
// when it creates the item (`validateTokenSecret` in
// packages/integrations/one-password/backend/src/install/validation.ts in the app). P0
// gives the connector a pointer to the secret on every call
// (packages/integrations/one-password/shared/src/token-secret.ts): on AWS the partial ARN
// arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>, and on Google
// Cloud the secret's project and name. P0 never holds the token.

var (
	// Google's rule for a secret's ID, which admits no `/`, so a full resource name
	// doesn't match.
	gcpSecretIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
	// How every 1Password service account token starts. The rest is letters, digits,
	// hyphens and underscores, which either secret manager's rule for a name would take.
	serviceAccountTokenPattern = regexp.MustCompile(`^ops_`)
)

// P0's messages for a token secret name it refuses. None repeats the value, which may
// be a token pasted in place of the name.
const (
	invalidSecretName  = "That doesn't look like a secret name. Enter the name of the secret that holds the service account token, not its ARN, its full resource name or the token itself."
	pastedToken        = "That's a 1Password service account token, not the name of its secret. Enter the name of the secret that holds the token. P0 never needs the token itself, so revoke it and create a new one if it was shared anywhere else."
	missingSecretName  = "Enter the name of the secret that holds, or will hold, the account's service account token."
	suffixedSecretName = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
)

// The rules for each secret manager type's fields, in the order P0 checks them.
var secretManagerRules = map[string][]fieldRules{
	awsSecretsManager: {
		{name: "account_id", rules: awsAccountRules(secretPlace)},
		{name: "secrets_region", rules: awsRegionRules(secretPlace)},
	},
	gcpSecretManager: {
		{name: "project_id", rules: gcpProjectRules(secretPlace)},
	},
}

// Refuses a 1Password service account token in place of its secret's name. It holds for a secret
// in any secret manager.
var notAToken = rule{
	summary: "1Password service account token in place of its secret's name",
	message: pastedToken,
	test:    func(name string) bool { return !serviceAccountTokenPattern.MatchString(name) },
}

// The rules for the name of a secret in each secret manager, in the order P0 checks them. A
// pasted token is refused first, since each secret manager's own rule would take one. The next
// says that it's no name at all.
var tokenSecretNameRules = map[string][]rule{
	awsSecretsManager: {
		notAToken,
		{
			summary: "Invalid service account token secret name",
			message: invalidSecretName,
			test:    awsSecretNamePattern.MatchString,
		},
		{
			summary: "Service account token secret name ends like an ARN suffix",
			message: suffixedSecretName,
			test: func(name string) bool {
				return !awsSecretNameSuffixPattern.MatchString(name)
			},
		},
	},
	gcpSecretManager: {
		notAToken,
		{
			summary: "Invalid service account token secret name",
			message: invalidSecretName,
			test:    gcpSecretIdPattern.MatchString,
		},
	},
}

// Rejects a secret manager block whose fields don't match its type, each secret manager field that P0
// refuses for where the secret is, and a secret name that P0 refuses for that secret manager. A
// secret manager of a type that isn't known yet is skipped, and so is the name's form, which
// depends on the secret manager. A pasted token and whitespace around the name don't, so they
// are refused even then: a token must never show in a plan or reach P0. P0 also refuses
// a secret manager outside the connector's cloud, which this can't check: it knows the connector
// only by its ID.
func validateTokenSecret(secretManager *secretManagerModel, name types.String, diags *diag.Diagnostics) {
	validateSecretManager(secretManager, diags)
	if secretManager != nil && isSet(secretManager.Type) {
		validateFields(path.Root("secret_manager"), secretManager.fields(), secretManagerRules[secretManager.Type.ValueString()], diags)
	}

	if !isSet(name) {
		return
	}
	// P0 checks that the name is there before it trims it.
	if name.ValueString() == "" {
		diags.AddAttributeError(path.Root("token_secret_name"), "Missing service account token secret name", missingSecretName)
		return
	}
	// Until the secret manager's type is known, only the rule that holds for any secret manager applies.
	rules := []rule{notAToken}
	if secretManager != nil {
		if ofSecretManager, ok := tokenSecretNameRules[secretManager.Type.ValueString()]; ok {
			rules = ofSecretManager
		}
	}
	validateField(path.Root("token_secret_name"), name, rules, diags)
}
