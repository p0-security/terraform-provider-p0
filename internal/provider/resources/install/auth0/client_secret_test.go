package installauth0

// copy of internal/provider/resources/install/docker/token_secret_test.go, renamed for
// Auth0 (INT-590), without the cases for a pasted Docker access token.

import (
	"strings"
	"testing"
)

// P0's message for the first of a vault's rules that name breaks, or "" when name
// breaks none, as validateField picks it.
func clientSecretNameProblem(vaultType string, name string) string {
	for _, r := range clientSecretNameRules[vaultType] {
		if !r.test(name) {
			return r.message
		}
	}
	return ""
}

// Each secret manager's rule for a secret's name and its edges: each value gets P0's
// message for the first rule it breaks.
func TestClientSecretNameRules(t *testing.T) {
	const aws, gcp = awsSecretsManager, gcpSecretManager

	cases := []struct {
		name  string
		vault string
		value string
		// P0's message, or "" for a name it accepts.
		want string
	}{
		{name: "a name", vault: aws, value: "auth0/acme/mgmt"},
		{name: "every character a name can have", vault: aws, value: "a/b_c+d=e.f@g-h"},
		{name: "a name at its longest", vault: aws, value: strings.Repeat("a", 512)},
		{name: "a name too long", vault: aws, value: strings.Repeat("a", 513), want: secretNameRule},
		{name: "empty", vault: aws, value: "", want: secretNameRule},
		// The connector reads the secret by its name in the vault's account and region.
		{name: "a full ARN", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:auth0-token-AbC123", want: secretNameRule},
		{name: "a partial ARN", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:auth0-token", want: secretNameRule},
		{name: "a Google secret's full resource name", vault: aws, value: "projects/acme-secrets/secrets/token:latest", want: secretNameRule},
		{name: "an inner space", vault: aws, value: "auth0 token", want: secretNameRule},
		{name: "a wildcard", vault: aws, value: "auth0/*", want: secretNameRule},
		// P0 checks the name's form before its suffix, so an ARN gets the form's message.
		{name: "an ARN ending like a suffix", vault: aws, value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:token-AbCdEf", want: secretNameRule},
		// Secrets Manager reads a name that ends this way as ending in a full ARN's suffix.
		{name: "a name ending like a suffix", vault: aws, value: "auth0/token-AbCdEf", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six digits", vault: aws, value: "auth0/acme/mgmt-123456", want: secretSuffixRule},
		{name: "a name ending in a hyphen and six letters", vault: aws, value: "acme/my-tenant", want: secretSuffixRule},
		{name: "a hyphen and five characters", vault: aws, value: "auth0-token-AbCdE"},
		{name: "a hyphen and seven characters", vault: aws, value: "auth0-token-AbCdEfG"},
		{name: "six characters after a slash", vault: aws, value: "auth0/token/AbCdEf"},
		// An Auth0 client secret fits the rule for a name, so P0 can't refuse one. That's
		// why no message repeats the value.
		{name: "a client secret", vault: aws, value: "Xy3_q-7AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefghijklmnopqrst"},

		{name: "a Google name", vault: gcp, value: "auth0-acme-mgmt"},
		{name: "every character a Google name can have", vault: gcp, value: "a_b-C1"},
		{name: "a Google name at its longest", vault: gcp, value: strings.Repeat("a", 255)},
		{name: "a Google name too long", vault: gcp, value: strings.Repeat("a", 256), want: secretNameRule},
		{name: "an empty Google name", vault: gcp, value: "", want: secretNameRule},
		{name: "a Google name's full resource name", vault: gcp, value: "projects/acme-secrets/secrets/auth0-token", want: secretNameRule},
		{name: "a Google name with a slash", vault: gcp, value: "auth0/acme/mgmt", want: secretNameRule},
		{name: "a Google name with a dot", vault: gcp, value: "auth0.token", want: secretNameRule},
		{name: "a Google name with a space", vault: gcp, value: "auth0 token", want: secretNameRule},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google name ending like a suffix", vault: gcp, value: "auth0-token-AbCdEf"},
		{name: "a client secret as a Google name", vault: gcp, value: "Xy3_q-7AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefghijklmnopqrst"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clientSecretNameProblem(c.vault, c.value); got != c.want {
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
		if len(clientSecretNameRules[vaultType]) == 0 {
			t.Errorf("no rules for a secret's name in a %s vault", vaultType)
		}
	}
}
