package installdocker

import (
	"strings"
	"testing"
)

// P0's message for the first of a vault's rules that name breaks, or "" when name
// breaks none, as validateField picks it.
func tokenSecretNameProblem(vaultType string, name string) string {
	for _, r := range tokenSecretNameRules[vaultType] {
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
		name  string
		vault string
		value string
		// P0's message, or "" for a name it accepts.
		want string
	}{
		{name: "a name", vault: aws, value: "docker/p0security/token"},
		{name: "every character a name can have", vault: aws, value: "a/b_c+d=e.f@g-h"},
		{name: "a name at its longest", vault: aws, value: strings.Repeat("a", 512)},
		{name: "a name too long", vault: aws, value: strings.Repeat("a", 513), want: secretNameRule},
		{name: "empty", vault: aws, value: "", want: secretNameRule},
		// The connector reads the secret by its name in the vault's account and region.
		{name: "a full ARN", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:docker-token-AbC123", want: secretNameRule},
		{name: "a partial ARN", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:docker-token", want: secretNameRule},
		{name: "a Google secret's full resource name", vault: aws, value: "projects/acme-secrets/secrets/token:latest", want: secretNameRule},
		{name: "an inner space", vault: aws, value: "docker token", want: secretNameRule},
		{name: "a wildcard", vault: aws, value: "docker/*", want: secretNameRule},
		// Checked before the suffix, which wouldn't help.
		{name: "an ARN ending like a suffix", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:token-AbCdEf", want: secretNameRule},
		// Secrets Manager can take a name that ends this way for the suffix of a full ARN.
		{name: "a name ending like a suffix", vault: aws, value: "docker/token-AbCdEf", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six digits", vault: aws, value: "docker/p0security/token-123456", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six letters", vault: aws, value: "p0security/my-docker", want: secretSuffixRule},
		{name: "a hyphen and five characters", vault: aws, value: "docker-token-AbCdE"},
		{name: "a hyphen and seven characters", vault: aws, value: "docker-token-AbCdEfG"},
		{name: "six characters after a slash", vault: aws, value: "docker/token/AbCdEf"},
		// A Docker access token fits the rule for a name, so it is refused before it.
		{name: "a token", vault: aws, value: "dckr_oat_example-Token_123", want: pastedTokenRule},
		{name: "a token of another kind", vault: aws, value: "dckr_PAT_x", want: pastedTokenRule},
		{name: "a token of no kind", vault: aws, value: "dckr_token", want: pastedTokenRule},
		{name: "a token ending like a suffix", vault: aws, value: "dckr_oat_example-AbCdEf", want: pastedTokenRule},
		{name: "a name with a token's start inside it", vault: aws, value: "docker/dckr_oat"},

		{name: "a Google name", vault: gcp, value: "docker-p0security-token"},
		{name: "every character a Google name can have", vault: gcp, value: "a_b-C1"},
		{name: "a Google name at its longest", vault: gcp, value: strings.Repeat("a", 255)},
		{name: "a Google name too long", vault: gcp, value: strings.Repeat("a", 256), want: secretNameRule},
		{name: "an empty Google name", vault: gcp, value: "", want: secretNameRule},
		{name: "a Google name's full resource name", vault: gcp, value: "projects/acme-secrets/secrets/docker-token", want: secretNameRule},
		{name: "a Google name with a slash", vault: gcp, value: "docker/p0security/token", want: secretNameRule},
		{name: "a Google name with a dot", vault: gcp, value: "docker.token", want: secretNameRule},
		{name: "a Google name with a space", vault: gcp, value: "docker token", want: secretNameRule},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google name ending like a suffix", vault: gcp, value: "docker-token-AbCdEf"},
		{name: "a token as a Google name", vault: gcp, value: "dckr_oat_example-Token_123", want: pastedTokenRule},
		{name: "a Google name with a token's start inside it", vault: gcp, value: "docker-dckr_oat"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tokenSecretNameProblem(c.vault, c.value); got != c.want {
				t.Errorf("problem with %q = %q; want %q", c.value, got, c.want)
			}
		})
	}
}

// Each vault type the attribute takes has rules for its fields and for a secret's name.
func TestEveryVaultTypeHasRules(t *testing.T) {
	for _, vaultType := range vaultTypes {
		if len(vaultRules[vaultType]) == 0 {
			t.Errorf("no rules for the fields of a %s vault", vaultType)
		}
		if len(tokenSecretNameRules[vaultType]) == 0 {
			t.Errorf("no rules for a secret's name in a %s vault", vaultType)
		}
	}
}
