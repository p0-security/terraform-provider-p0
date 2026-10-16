package installonepassword

import (
	"strings"
	"testing"
)

// P0's message for the first of a secret manager's rules that name breaks, or "" when name
// breaks none, as validateField picks it.
func tokenSecretNameProblem(secretManagerType string, name string) string {
	for _, r := range tokenSecretNameRules[secretManagerType] {
		if !r.test(name) {
			return r.message
		}
	}
	return ""
}

// Each secret manager's rule for a secret's name and its edges: each value gets P0's
// message for the first rule it breaks.
func TestTokenSecretNameRules(t *testing.T) {
	const aws, gcp = awsSecretsManager, gcpSecretManager

	cases := []struct {
		name          string
		secretManager string
		value         string
		// P0's message, or "" for a name it accepts.
		want string
	}{
		{name: "a name", secretManager: aws, value: "1password/p0security/token"},
		{name: "every character a name can have", secretManager: aws, value: "a/b_c+d=e.f@g-h"},
		{name: "a name at its longest", secretManager: aws, value: strings.Repeat("a", 512)},
		{name: "a name too long", secretManager: aws, value: strings.Repeat("a", 513), want: secretNameRule},
		{name: "empty", secretManager: aws, value: "", want: secretNameRule},
		// The connector reads the secret by its name in the secret manager's account and region.
		{name: "a full ARN", secretManager: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:1password-token-AbC123", want: secretNameRule},
		{name: "a partial ARN", secretManager: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:1password-token", want: secretNameRule},
		{name: "a Google secret's full resource name", secretManager: aws, value: "projects/acme-secrets/secrets/token:latest", want: secretNameRule},
		{name: "an inner space", secretManager: aws, value: "1password token", want: secretNameRule},
		{name: "a wildcard", secretManager: aws, value: "1password/*", want: secretNameRule},
		// Checked before the suffix, which wouldn't help.
		{name: "an ARN ending like a suffix", secretManager: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:token-AbCdEf", want: secretNameRule},
		// Secrets Manager can take a name that ends this way for the suffix of a full ARN.
		{name: "a name ending like a suffix", secretManager: aws, value: "1password/token-AbCdEf", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six digits", secretManager: aws, value: "1password/p0security/token-123456", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six letters", secretManager: aws, value: "p0security/my-vaults", want: secretSuffixRule},
		{name: "a hyphen and five characters", secretManager: aws, value: "1password-token-AbCdE"},
		{name: "a hyphen and seven characters", secretManager: aws, value: "1password-token-AbCdEfG"},
		{name: "six characters after a slash", secretManager: aws, value: "1password/token/AbCdEf"},
		// A 1Password service account token fits the rule for a name, so it is refused before it.
		{name: "a token", secretManager: aws, value: "ops_example-Token_123", want: pastedTokenRule},
		{name: "a token of another kind", secretManager: aws, value: "ops_eyJzaWduSW5BZGRyZXNz", want: pastedTokenRule},
		{name: "a token of no kind", secretManager: aws, value: "ops_", want: pastedTokenRule},
		{name: "a token ending like a suffix", secretManager: aws, value: "ops_example-AbCdEf", want: pastedTokenRule},
		{name: "a name with a token's start inside it", secretManager: aws, value: "1password/ops_x"},

		{name: "a Google name", secretManager: gcp, value: "1password-p0security-token"},
		{name: "every character a Google name can have", secretManager: gcp, value: "a_b-C1"},
		{name: "a Google name at its longest", secretManager: gcp, value: strings.Repeat("a", 255)},
		{name: "a Google name too long", secretManager: gcp, value: strings.Repeat("a", 256), want: secretNameRule},
		{name: "an empty Google name", secretManager: gcp, value: "", want: secretNameRule},
		{name: "a Google name's full resource name", secretManager: gcp, value: "projects/acme-secrets/secrets/1password-token", want: secretNameRule},
		{name: "a Google name with a slash", secretManager: gcp, value: "1password/p0security/token", want: secretNameRule},
		{name: "a Google name with a dot", secretManager: gcp, value: "1password.token", want: secretNameRule},
		{name: "a Google name with a space", secretManager: gcp, value: "1password token", want: secretNameRule},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google name ending like a suffix", secretManager: gcp, value: "1password-token-AbCdEf"},
		{name: "a token as a Google name", secretManager: gcp, value: "ops_example-Token_123", want: pastedTokenRule},
		{name: "a Google name with a token's start inside it", secretManager: gcp, value: "1password-ops_x"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tokenSecretNameProblem(c.secretManager, c.value); got != c.want {
				t.Errorf("problem with %q = %q; want %q", c.value, got, c.want)
			}
		})
	}
}

// Each secret manager type the attribute takes has rules for its fields and for a secret's name.
func TestEverySecretManagerTypeHasRules(t *testing.T) {
	for _, secretManagerType := range secretManagerTypes {
		if len(secretManagerRules[secretManagerType]) == 0 {
			t.Errorf("no rules for the fields of a %s secret manager", secretManagerType)
		}
		if len(tokenSecretNameRules[secretManagerType]) == 0 {
			t.Errorf("no rules for a secret's name in a %s secret manager", secretManagerType)
		}
	}
}
