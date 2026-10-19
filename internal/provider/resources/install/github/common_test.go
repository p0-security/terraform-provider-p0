package installgithubrepositories

import (
	"strings"
	"testing"
)

func TestGithubOrgRegex(t *testing.T) {
	cases := map[string]bool{
		"my-github-org":         true,
		"acme":                  true,
		"A1":                    true,
		"0rg":                   true,
		strings.Repeat("a", 39): true,
		strings.Repeat("a", 40): false,
		"-acme":                 false,
		"acme_corp":             false,
		"acme/api":              false,
		"acme.corp":             false,
		"":                      false,
	}

	for org, want := range cases {
		if got := GithubOrgRegex.MatchString(org); got != want {
			t.Errorf("GithubOrgRegex.MatchString(%q) = %v, want %v", org, got, want)
		}
	}
}

func TestGithubAppIdRegex(t *testing.T) {
	cases := map[string]bool{
		"123456":     true,
		"1":          true,
		"0123":       true,
		"":           false,
		"-1":         false,
		"12.5":       false,
		"Iv1.abc123": false,
		"123 456":    false,
		"123456\n":   false,
	}

	for appId, want := range cases {
		if got := githubAppIdRegex.MatchString(appId); got != want {
			t.Errorf("githubAppIdRegex.MatchString(%q) = %v, want %v", appId, got, want)
		}
	}
}
