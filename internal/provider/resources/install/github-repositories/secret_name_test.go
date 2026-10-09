package installgithubrepositories

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// P0's message for the first of rules that name breaks, or "" when name breaks none,
// as validateField picks it.
func secretNameProblem(name string, rules []rule) string {
	for _, r := range rules {
		if !r.test(name) {
			return r.message
		}
	}
	return ""
}

// Each AWS rule and its edges: each value gets P0's message for the first rule it
// breaks.
func TestSecretNameRules(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"

	cases := []struct {
		name  string
		value string
		// P0's message, or "" for a name it accepts.
		want string
	}{
		{name: "a name", value: "github/acme/private-key"},
		{name: "every character a name can have", value: "a/b_c+d=e.f@g-h"},
		{name: "a name at its longest", value: strings.Repeat("a", 512)},
		{name: "a name too long", value: strings.Repeat("a", 513), want: invalidSecretName},
		{name: "empty", value: "", want: invalidSecretName},
		// The connector reads the secret by its name in the vault's account and region.
		{name: "a full ARN", value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:github-key-AbC123", want: invalidSecretName},
		{name: "a partial ARN", value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:github-key", want: invalidSecretName},
		{name: "a GovCloud ARN", value: "arn:aws-us-gov:secretsmanager:us-gov-west-1:111111111111:secret:key", want: invalidSecretName},
		{name: "a Google secret", value: "projects/acme-project/secrets/key:latest", want: invalidSecretName},
		{name: "a pasted key", value: pem, want: invalidSecretName},
		{name: "an inner space", value: "private key", want: invalidSecretName},
		{name: "a wildcard", value: "github/*", want: invalidSecretName},
		// Checked before the suffix, which wouldn't help.
		{name: "an ARN ending like a suffix", value: "arn:aws:secretsmanager:us-west-2:111111111111:secret:key-AbCdEf", want: invalidSecretName},
		// Secrets Manager can take a name that ends this way for the suffix of a full ARN.
		{name: "a name ending like a suffix", value: "github-key-AbCdEf", want: suffixedSecretName},
		{name: "a name ending in a hyphen and six digits", value: "github/acme/key-123456", want: suffixedSecretName},
		{name: "a name ending in a hyphen and six letters", value: "acme/my-github", want: suffixedSecretName},
		{name: "a suffix alone", value: "-AbCdEf", want: suffixedSecretName},
		{name: "a hyphen and five characters", value: "github-key-AbCdE"},
		{name: "a hyphen and seven characters", value: "github-key-AbCdEfG"},
		{name: "six characters after an underscore", value: "github_key_AbCdEf"},
		{name: "six characters after a slash", value: "github/key/AbCdEf"},
		{name: "a hyphen and six characters with an underscore", value: "github-key-AbC_Ef"},
		{name: "six characters alone", value: "AbCdEf"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := secretNameProblem(c.value, secretNameRules[AwsSecretsManager]); got != c.want {
				t.Errorf("problem with %q = %q; want %q", c.value, got, c.want)
			}
		})
	}
}

// Google's rule and its edges. A Google secret's name has no slash, and no suffix.
func TestGcpSecretNameRules(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"

	cases := []struct {
		name  string
		value string
		// P0's message, or "" for a name it accepts.
		want string
	}{
		{name: "a name", value: "github-acme-private-key"},
		{name: "every character a name can have", value: "a_b-C9"},
		{name: "a name at its longest", value: strings.Repeat("a", 255)},
		{name: "a name too long", value: strings.Repeat("a", 256), want: invalidGcpSecretName},
		{name: "empty", value: "", want: invalidGcpSecretName},
		// Only AWS reads a name that ends this way as a suffix.
		{name: "a name ending like an ARN's suffix", value: "github-key-AbCdEf"},
		// The connector reads the secret by its name in the vault's project.
		{name: "a resource name", value: "projects/acme-project/secrets/github-key", want: invalidGcpSecretName},
		{name: "an AWS name with a slash", value: "github/acme/private-key", want: invalidGcpSecretName},
		{name: "a name with a dot", value: "github.key", want: invalidGcpSecretName},
		{name: "a pasted key", value: pem, want: invalidGcpSecretName},
		{name: "a wildcard", value: "github-*", want: invalidGcpSecretName},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := secretNameProblem(c.value, secretNameRules[GcpSecretManager]); got != c.want {
				t.Errorf("problem with %q = %q; want %q", c.value, got, c.want)
			}
		})
	}
}

// Before the vault's type is known, a name either vault takes passes.
func TestSecretNameRulesForAnUnknownVault(t *testing.T) {
	unknown := &vaultModel{Type: types.StringUnknown()}

	cases := []struct {
		name  string
		vault *vaultModel
		value string
		want  string
	}{
		{name: "an AWS name", vault: unknown, value: "github/acme/private-key"},
		{name: "a Google name that ends like an AWS suffix", vault: unknown, value: "github-key-AbCdEf"},
		{name: "a name no vault takes", vault: unknown, value: "github key", want: invalidSecretName},
		{name: "no vault at all", vault: nil, value: "github key", want: invalidSecretName},
		{name: "a known Google vault", vault: &vaultModel{Type: types.StringValue(GcpSecretManager)}, value: "github/acme/private-key", want: invalidGcpSecretName},
		{name: "a known AWS vault", vault: &vaultModel{Type: types.StringValue(AwsSecretsManager)}, value: "github-key-AbCdEf", want: suffixedSecretName},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := secretNameProblem(c.value, secretNameRulesFor(c.vault)); got != c.want {
				t.Errorf("problem with %q = %q; want %q", c.value, got, c.want)
			}
		})
	}
}
