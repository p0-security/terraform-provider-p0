package installdatabricks

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestCommercialRegionRegex(t *testing.T) {
	cases := map[string]bool{
		"us-east-1":      true,
		"us-west-2":      true,
		"eu-central-2":   true,
		"ap-southeast-7": true,
		"ca-west-1":      true,
		"mx-central-1":   true,
		"il-central-1":   true,
		"us-gov-west-1":  false,
		"us-gov-east-1":  false,
		"cn-north-1":     false,
		"us-isob-east-1": false,
		"us-west":        false,
		"":               false,
	}

	for region, want := range cases {
		if got := CommercialRegionRegex.MatchString(region); got != want {
			t.Errorf("CommercialRegionRegex.MatchString(%q) = %v, want %v", region, got, want)
		}
	}
}

// The catalog names that the app's validator accepts and rejects.
func TestCatalogNameValidators(t *testing.T) {
	cases := map[string]bool{
		"main":                   true,
		"sales_prod":             true,
		"sales-prod":             true,
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
		// Unity Catalog stores names in lowercase.
		"Sales":   false,
		"DONNÉES": false,
		// P0's install API carries item keys in its URL paths unescaped.
		"sales#eu": false,
		"sales?eu": false,
		"a%62c":    false,
		`a\b`:      false,
	}

	for name, want := range cases {
		valid := true
		for _, v := range catalogNameValidators() {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("catalog_name"),
				ConfigValue: types.StringValue(name),
			}, resp)
			if resp.Diagnostics.HasError() {
				valid = false
			}
		}
		if valid != want {
			t.Errorf("catalog name %q valid = %v, want %v", name, valid, want)
		}
	}
}

func TestParseCatalogKey(t *testing.T) {
	cases := []struct {
		key, catalogName, workspaceId string
		ok                            bool
	}{
		{"main@1234567890123456", "main", "1234567890123456", true},
		// Unity Catalog allows "@" in a name; the workspace ID never has one.
		{"a@b@7", "a@b", "7", true},
		{"main", "", "", false},
		{"@7", "", "", false},
		{"main@", "", "", false},
		{"main@dbc-1234", "", "", false},
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
