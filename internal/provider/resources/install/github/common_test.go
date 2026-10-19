package installgithub

import (
	"strings"
	"testing"
)

func TestGithubOrgPattern(t *testing.T) {
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
		if got := GithubOrgPattern.MatchString(org); got != want {
			t.Errorf("GithubOrgPattern.MatchString(%q) = %v, want %v", org, got, want)
		}
	}
}

func TestGithubAppIdPattern(t *testing.T) {
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
		if got := githubAppIdPattern.MatchString(appId); got != want {
			t.Errorf("githubAppIdPattern.MatchString(%q) = %v, want %v", appId, got, want)
		}
	}
}
