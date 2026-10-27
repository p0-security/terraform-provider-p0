package installtailscale

import (
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// copy of internal/provider/resources/install/auth0/client_secret.go @ 663ab81
// (INT-590), renamed for Tailscale (INT-686), with Tailscale's messages and the paths of
// the tailnet's credential.
//
// P0's rules for where an OAuth client's secret is, in the customer's own secret
// manager: the vault's fields and the secret's name, as P0 checks them when it creates
// the item (`validateClientSecret` in
// packages/integrations/tailscale/backend/src/install/validation.ts in the app). On
// every call P0 tells the connector where the secret is
// (packages/integrations/tailscale/shared/src/credential.ts): on AWS the partial ARN
// arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>, and on Google
// Cloud the secret's project and name. P0 never holds the secret.

// Google's rule for a secret's ID, which admits no `/`, so a full resource name doesn't
// match.
var gcpSecretIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// P0's messages for a secret name it refuses (INVALID_SECRET_NAME, MISSING_SECRET_NAME
// and SUFFIXED_SECRET_NAME). None repeats the value, because a customer may have pasted
// the OAuth client's secret in place of the name.
const (
	invalidSecretName  = "That doesn't look like a secret name. Enter the name of the secret that holds the OAuth client's secret, not its ARN, its full resource name or the client secret itself."
	missingSecretName  = "Enter the name of the secret that holds, or will hold, the OAuth client's secret."
	suffixedSecretName = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
)

// Tailscale's secrets, such as an OAuth client's tskey-client-..., start with this, in
// any case.
const tailscaleSecretPrefix = "tskey-"

// The messages for a Tailscale secret entered as a client ID or as a secret's name,
// in the app's words (PASTED_SECRET_FOR_CLIENT_ID and PASTED_SECRET_FOR_NAME).
// Neither repeats the value.
const (
	tailscaleSecretAsClientId   = "That's a Tailscale secret, which starts with tskey-, not a client ID. Enter the client ID, as Tailscale's Trust credentials page shows it."
	tailscaleSecretAsSecretName = "That's a Tailscale secret, which starts with tskey-, not a secret's name. Put the OAuth client's secret in your secret manager, and enter the name of the secret that holds it."
)

// Refuses a Tailscale secret, which would otherwise pass the rules for a client ID and
// for a secret's name, and show in plan output and state.
func notTailscaleSecret(summary, message string) rule {
	return rule{
		summary: summary,
		message: message,
		test: func(value string) bool {
			return !strings.HasPrefix(strings.ToLower(value), tailscaleSecretPrefix)
		},
	}
}

// Where the secret's name is in the tailnet's configuration.
var clientSecretNamePath = credentialPath.AtName("client_secret_name")

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

// The rules for a secret's name in each vault, in the order P0 checks them. A Tailscale
// OAuth client's secret is letters, digits and `-`, which either vault's rule accepts,
// so validateClientSecret refuses one by its prefix before these, and no message
// repeats the value.
var clientSecretNameRules = map[string][]rule{
	awsSecretsManager: {
		{
			summary: "Invalid secret name for the OAuth client's secret",
			message: invalidSecretName,
			test:    awsSecretNamePattern.MatchString,
		},
		{
			summary: "Secret name for the OAuth client's secret ends like an ARN suffix",
			message: suffixedSecretName,
			test: func(name string) bool {
				return !awsSecretNameSuffixPattern.MatchString(name)
			},
		},
	},
	gcpSecretManager: {
		{
			summary: "Invalid secret name for the OAuth client's secret",
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
func validateClientSecret(vault *vaultModel, name types.String, diags *diag.Diagnostics) {
	validateVault(vault, diags)
	if vault != nil && isSet(vault.Type) {
		validateFields(vaultPath, vault.fields(), vaultRules[vault.Type.ValueString()], diags)
	}

	if !isSet(name) {
		return
	}
	// P0 checks that the name is there before it trims it.
	if name.ValueString() == "" {
		diags.AddAttributeError(clientSecretNamePath, "Missing secret name for the OAuth client's secret", missingSecretName)
		return
	}
	// A pasted Tailscale secret is refused whatever the vault, so even before its type
	// is known.
	rules := []rule{notTailscaleSecret("Tailscale secret entered as the secret's name", tailscaleSecretAsSecretName)}
	if vault != nil {
		rules = append(rules, clientSecretNameRules[vault.Type.ValueString()]...)
	}
	validateField(clientSecretNamePath, name, rules, diags)
}
