package installdatabricks

import (
	"strings"
	"testing"
)

func TestWorkspaceIdRegex(t *testing.T) {
	cases := map[string]bool{
		"1234567890123456":  true,
		"7":                 true,
		"":                  false,
		"dbc-1234abcd-5678": false,
		"123 456":           false,
		"-1":                false,
	}

	for id, want := range cases {
		if got := WorkspaceIdRegex.MatchString(id); got != want {
			t.Errorf("WorkspaceIdRegex.MatchString(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestCatalogNameRegex(t *testing.T) {
	cases := map[string]bool{
		"main":                   true,
		"sales_prod":             true,
		"sales-prod":             true,
		"Sales":                  true,
		"données":                true,
		strings.Repeat("a", 255): true,
		strings.Repeat("a", 256): false,
		"":                       false,
		// The connector splits full names on periods.
		"main.default": false,
		"my catalog":   false,
		"a/b":          false,
		"tab\there":    false,
		"del\x7f":      false,
	}

	for name, want := range cases {
		if got := CatalogNameRegex.MatchString(name); got != want {
			t.Errorf("CatalogNameRegex.MatchString(%q) = %v, want %v", name, got, want)
		}
	}
}
