package installharbor

import (
	"regexp"

	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// P0's rules for the name of the secret that holds the connector robot account's
// credentials, and its messages for a name it refuses. The name is a secret's name in
// the instance's vault, never an ARN or a resource name: P0 builds the secret's
// address from the name and the vault.
//
// The app's `secretName` field checks AWS Secrets Manager's rule, which every Google
// Secret Manager name fits too, since the field can't see the vault
// (packages/integrations/harbor/shared/src/secret-name.ts). The provider sees the
// vault, so it checks a name in Google Secret Manager against that vault's rule, which
// the Harbor connector also applies to the secret's ID. The app has no message for
// that rule today. This one is from the first version of the app's file, and the
// instance's installer (INT-394) is to keep it.
//
// TODO(INT-417): awsSecretNameRegex and its message's first sentence are also in
// github-repositories' secret_name.go.
var (
	// Neither pattern admits whitespace, a colon, a quote or a wildcard, which keeps
	// out an ARN, a wildcard and the robot account's JSON.
	awsSecretNameRegex = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	gcpSecretNameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
)

const (
	invalidSecretNameSummary = "Invalid robot account secret name"
	invalidSecretName        = "That doesn't look like a secret name. Enter the name of the secret that holds the connector robot account's credentials, " +
		"not its ARN or the credentials themselves."
	invalidGcpSecretName = "That isn't a secret's name in Google Secret Manager. Enter the name alone, without its project or a version: " +
		"letters, digits, underscores and hyphens, up to 255 characters."
)

// P0's rule for the name of the robot account's secret in a vault of vaultType. Its
// message doesn't repeat the name, which may be the credentials pasted in its place. A
// vault whose type isn't known yet takes any name that a vault of some type would.
func secretNameRules(vaultType string) []rule {
	if vaultType == installvaultedcredential.GcpSecretManager {
		return []rule{{summary: invalidSecretNameSummary, message: invalidGcpSecretName, test: gcpSecretNameRegex.MatchString}}
	}
	return []rule{{summary: invalidSecretNameSummary, message: invalidSecretName, test: awsSecretNameRegex.MatchString}}
}
