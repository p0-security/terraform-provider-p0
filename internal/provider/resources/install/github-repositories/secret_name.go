package installgithubrepositories

import (
	"regexp"
	"strings"
)

// The forms each vault accepts for the private key's secret, as P0 checks them
// (packages/integrations/github-repositories-shared/src/secret-name.ts in the app):
// AWS takes a secret's name or its ARN. Neither allows whitespace, so a pasted key
// never matches.
var secretNamePatterns = map[string][]*regexp.Regexp{
	AwsSecretsManager: {
		regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`),
		regexp.MustCompile(`^arn:aws[a-z-]*:secretsmanager:[a-z0-9-]+:\d{12}:secret:[A-Za-z0-9/_+=.@-]+$`),
	},
}

// P0's message for a private key secret name it refuses.
const InvalidSecretName = "That doesn't look like a secret name. Enter the name or ARN of the secret that holds the key, not the key itself."

// Whether name names a secret in a vault of type vaultType, or in any vault when the
// type isn't known. A PEM header alone has no whitespace, so it's refused
// explicitly.
func isSecretName(name string, vaultType string) bool {
	if strings.Contains(name, "-----BEGIN") {
		return false
	}
	patterns, known := secretNamePatterns[vaultType]
	if !known {
		for _, vaultPatterns := range secretNamePatterns {
			patterns = append(patterns, vaultPatterns...)
		}
	}
	for _, pattern := range patterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}
