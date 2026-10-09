package installgithubrepositories

import (
	"regexp"

	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// P0's rules for the private key's secret, as it checks them
// (packages/integrations/github-repositories/shared/src/secret-name.ts in the app): the
// secret's name alone, as its vault names it. P0 has the connector read, on AWS, the
// partial ARN arn:aws:secretsmanager:<secrets_region>:<vault account>:secret:<name>,
// which Secrets Manager resolves to the secret of that name, and on Google Cloud, the
// resource name projects/<vault project>/secrets/<name>.

var (
	// A Secrets Manager secret's name. It admits no whitespace and no colon, so neither
	// a pasted key nor an ARN matches.
	secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	// The hyphen and six letters or digits that Secrets Manager adds to a secret's name
	// in its ARN. In a partial ARN, Secrets Manager can take a name that ends this way
	// for that suffix, and then doesn't find the secret.
	secretNameSuffixPattern = regexp.MustCompile(`-[A-Za-z0-9]{6}$`)
	// A Secret Manager secret's name, its ID. It admits no whitespace and no slash, so
	// neither a pasted key nor a resource name matches.
	gcpSecretNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
)

// P0's messages for a private key secret name it refuses. None repeats the value,
// which may be a key pasted in place of the name.
const (
	InvalidSecretName    = "That doesn't look like a secret name. Enter the name of the secret that holds the key, not its ARN or the key itself."
	InvalidGcpSecretName = "That doesn't look like a secret name. Enter the name of the secret that holds the key, not its resource name or the key itself."
	SuffixedSecretName   = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
)

// P0's rules for the private key's secret name in each vault, in the order it checks
// them.
var secretNameRules = map[string][]rule{
	AwsSecretsManager: {
		{
			summary: "Invalid private key secret name",
			message: InvalidSecretName,
			test:    secretNamePattern.MatchString,
		},
		{
			summary: "Private key secret name ends like an ARN suffix",
			message: SuffixedSecretName,
			test:    func(name string) bool { return !secretNameSuffixPattern.MatchString(name) },
		},
	},
	GcpSecretManager: {
		{
			summary: "Invalid private key secret name",
			message: InvalidGcpSecretName,
			test:    gcpSecretNamePattern.MatchString,
		},
	},
}

// Whether name breaks none of rules.
func passes(name string, rules []rule) bool {
	for _, r := range rules {
		if !r.test(name) {
			return false
		}
	}
	return true
}

// P0's rules for the private key's secret name in vault. While the vault's type isn't
// known, as when it's interpolated, a name that either vault takes passes, as on P0's
// form, and P0 checks it against the vault when it creates the installation.
func secretNameRulesFor(vault *vaultModel) []rule {
	if vault != nil && installapp.IsSet(vault.Type) {
		if rules, ok := secretNameRules[vault.Type.ValueString()]; ok {
			return rules
		}
	}
	return []rule{{
		summary: "Invalid private key secret name",
		message: InvalidSecretName,
		test: func(name string) bool {
			return passes(name, secretNameRules[AwsSecretsManager]) || passes(name, secretNameRules[GcpSecretManager])
		},
	}}
}
