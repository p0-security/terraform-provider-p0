package installoktaawsiamfederation

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// copy of internal/provider/resources/install/auth0/client_secret.go, renamed for Okta (AWS IAM
// Federation) (INT-1473).
//
// P0's rules for where an app's private key is, in the customer's own secret
// manager: the vault's fields and the secret's name, as P0 checks them when it creates
// the item (`validatePrivateKey` in
// packages/integrations/okta-aws-iam-federation/backend/src/install/validation.ts in the app). On every
// call P0 tells the connector where the secret is
// (packages/integrations/okta-aws-iam-federation/shared/src/private-key.ts): on AWS the partial ARN
// arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>, and on Google
// Cloud the secret's project and name. P0 never holds the private key.

// Google's rule for a secret's ID, which admits no `/`, so a full resource name doesn't
// match.
var gcpSecretIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// P0's messages for a secret name it refuses. None repeats the value, because a
// customer may have pasted the private key in place of the name.
const (
	invalidSecretName  = "That doesn't look like a secret name. Enter the name of the secret that holds the private key, not its ARN, its full resource name or the private key itself."
	missingSecretName  = "Enter the name of the secret that holds, or will hold, the private key."
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

// The rules for a secret's name in each vault, in the order P0 checks them. Nothing can tell a pasted private key from a name, so no message repeats the value.
var privateKeyNameRules = map[string][]rule{
	awsSecretsManager: {
		{
			summary: "Invalid secret name for the private key",
			message: invalidSecretName,
			test:    awsSecretNamePattern.MatchString,
		},
		{
			summary: "Secret name for the private key ends like an ARN suffix",
			message: suffixedSecretName,
			test: func(name string) bool {
				return !awsSecretNameSuffixPattern.MatchString(name)
			},
		},
	},
	gcpSecretManager: {
		{
			summary: "Invalid secret name for the private key",
			message: invalidSecretName,
			test:    gcpSecretIdPattern.MatchString,
		},
	},
}

// Rejects a vault block whose fields don't match its type, each vault field that P0
// refuses for where the secret is, and a secret name that P0 refuses for that vault.
// Until the vault's type is known, this skips the vault's fields and the name's form,
// which depends on the vault. Whitespace around the name doesn't depend on the vault,
// so this refuses it even then. P0 also refuses a vault outside the connector's cloud,
// which this can't check, because it knows the connector only by its ID.
func validatePrivateKey(vault *vaultModel, name types.String, diags *diag.Diagnostics) {
	validateVault(vault, diags)
	if vault != nil && isSet(vault.Type) {
		validateFields(path.Root("vault"), vault.fields(), vaultRules[vault.Type.ValueString()], diags)
	}

	if !isSet(name) {
		return
	}
	// P0 checks that the name is there before it trims it.
	if name.ValueString() == "" {
		diags.AddAttributeError(path.Root("private_key_name"), "Missing secret name for the private key", missingSecretName)
		return
	}
	var rules []rule
	if vault != nil {
		rules = privateKeyNameRules[vault.Type.ValueString()]
	}
	validateField(path.Root("private_key_name"), name, rules, diags)
}
