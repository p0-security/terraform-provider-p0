package installgithubrepositories

import (
	"regexp"
)

// P0's rules for the private key's secret, as it checks them
// (packages/integrations/github-repositories-shared/src/secret-name.ts in the app): the
// secret's name alone. P0 has the connector read the partial ARN
// arn:aws:secretsmanager:<secrets_region>:<vault account>:secret:<name>, which Secrets
// Manager resolves to the secret of that name.

var (
	// A Secrets Manager secret's name. It admits no whitespace and no colon, so neither
	// a pasted key nor an ARN matches.
	secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	// The hyphen and six letters or digits that Secrets Manager adds to a secret's name
	// in its ARN. In a partial ARN, Secrets Manager can take a name that ends this way
	// for that suffix, and then doesn't find the secret.
	secretNameSuffixPattern = regexp.MustCompile(`-[A-Za-z0-9]{6}$`)
)

// P0's messages for a private key secret name it refuses. Neither repeats the value,
// which may be a key pasted in place of the name.
const (
	InvalidSecretName  = "That doesn't look like a secret name. Enter the name of the secret that holds the key, not its ARN or the key itself."
	SuffixedSecretName = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
)

// P0's rules for the private key's secret name, in the order it checks them.
var secretNameRules = []rule{
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
}
