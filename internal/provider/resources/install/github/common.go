package installgithubrepositories

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const GithubRepositoriesKey = "github-repositories"

// All installable GitHub Repositories components.
var Components = []string{installresources.RepositoryAccess}

// P0's rules for the item's ID and its App, and its messages for a value it refuses
// (packages/integrations/github-repositories-shared/src/components.ts in the app).

// A GitHub organization login: letters, digits and hyphens, not starting with a hyphen,
// at most 39 characters. This is P0's rule for the item ID (GITHUB_ORG_PATTERN), which
// the connector applies too, so a login P0 would refuse fails at plan time instead.
var GithubOrgRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)

const InvalidOrgLogin = "Enter the organization's GitHub login, as in github.com/<login>"

// A GitHub App's ID, the number on the App's settings page (GITHUB_APP_ID_PATTERN).
var githubAppIdRegex = regexp.MustCompile(`^\d+$`)

const InvalidAppId = "The GitHub App ID is a number. Find it on the app's settings page."
