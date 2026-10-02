package installgithubapp

import (
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const GithubAppKey = "github-app"

// All installable GitHub App components.
var Components = []string{installresources.Credential}

const (
	integrationLabel = "GitHub App"
	secretLabel      = "the GitHub App's private key"
)
