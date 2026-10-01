package installgithubrepositories

import (
	"strings"
	"testing"
)

// The cases P0's own tests check, plus each form's edges.
func TestIsSecretName(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"
	const arn = "arn:aws:secretsmanager:us-west-2:111111111111:secret:github-key"

	cases := []struct {
		name      string
		value     string
		vaultType string
		want      bool
	}{
		{name: "aws name", value: "github/acme/private-key", vaultType: AwsSecretsManager, want: true},
		{name: "aws name at its longest", value: strings.Repeat("a", 512), vaultType: AwsSecretsManager, want: true},
		{name: "aws name too long", value: strings.Repeat("a", 513), vaultType: AwsSecretsManager, want: false},
		{name: "aws arn", value: arn, vaultType: AwsSecretsManager, want: true},
		{name: "aws govcloud arn", value: "arn:aws-us-gov:secretsmanager:us-gov-west-1:111111111111:secret:key", vaultType: AwsSecretsManager, want: true},
		{name: "aws arn with a short account", value: "arn:aws:secretsmanager:us-west-2:1111:secret:key", vaultType: AwsSecretsManager, want: false},
		{name: "aws name without a vault", value: "github/acme/private-key", want: true},
		{name: "aws arn without a vault", value: arn, want: true},
		{name: "pasted key", value: pem, vaultType: AwsSecretsManager, want: false},
		{name: "pasted key without a vault", value: pem, want: false},
		{name: "pem header alone", value: "-----BEGIN", vaultType: AwsSecretsManager, want: false},
		{name: "inner space", value: "private key", vaultType: AwsSecretsManager, want: false},
		{name: "trailing newline", value: "private-key\n", vaultType: AwsSecretsManager, want: false},
		{name: "empty", value: "", vaultType: AwsSecretsManager, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isSecretName(c.value, c.vaultType); got != c.want {
				t.Errorf("isSecretName(%q, %q) = %v; want %v", c.value, c.vaultType, got, c.want)
			}
		})
	}
}
