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

// The catalog names that the app's catalogNameError refuses, with its messages.
// An empty message means the name is valid.
func TestCatalogNameError(t *testing.T) {
	const (
		tooLong     = "Catalog names are at most 255 characters"
		forbidden   = "Catalog names can't contain a period, a space, a forward slash or a control character"
		urlPathOnly = `P0 can't install a catalog whose name contains #, ?, % or \`
	)
	cases := map[string]string{
		"main":                   "",
		"sales_prod":             "",
		"sales-prod":             "",
		"données":                "",
		strings.Repeat("a", 255): "",
		strings.Repeat("a", 256): tooLong,
		// JavaScript counts a character outside the BMP as two, and so does the app.
		strings.Repeat("😀", 127) + "a": "",
		strings.Repeat("😀", 128):       tooLong,
		"":                             "Enter the catalog's name",
		// The connector splits full names on periods.
		"main.default": forbidden,
		"my catalog":   forbidden,
		"a/b":          forbidden,
		"tab\there":    forbidden,
		"del\x7f":      forbidden,
		// Unity Catalog stores names in lowercase.
		"Sales":   "Unity Catalog stores catalog names in lowercase, so enter sales",
		"DONNÉES": "Unity Catalog stores catalog names in lowercase, so enter données",
		// P0's install API carries item keys in its URL paths unescaped.
		"sales#eu": urlPathOnly,
		"sales?eu": urlPathOnly,
		"a%62c":    urlPathOnly,
		`a\b`:      urlPathOnly,
	}

	for name, want := range cases {
		if got := catalogNameError(name); got != want {
			t.Errorf("catalogNameError(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestParseCatalogKey(t *testing.T) {
	cases := []struct {
		key, catalogName, workspaceId string
		ok                            bool
	}{
		{key: "main@1234567890123456", catalogName: "main", workspaceId: "1234567890123456", ok: true},
		// Unity Catalog allows "@" in a name; the workspace ID never has one.
		{key: "a@b@7", catalogName: "a@b", workspaceId: "7", ok: true},
		{key: "main"},
		{key: "@7"},
		{key: "main@"},
		{key: "main@dbc-1234"},
	}

	for _, c := range cases {
		catalogName, workspaceId, ok := parseCatalogKey(c.key)
		if catalogName != c.catalogName || workspaceId != c.workspaceId || ok != c.ok {
			t.Errorf("parseCatalogKey(%q) = %q, %q, %v, want %q, %q, %v", c.key, catalogName, workspaceId, ok, c.catalogName, c.workspaceId, c.ok)
		}
		if ok && catalogKey(catalogName, workspaceId) != c.key {
			t.Errorf("catalogKey(%q, %q) = %q, want %q", catalogName, workspaceId, catalogKey(catalogName, workspaceId), c.key)
		}
	}
}
