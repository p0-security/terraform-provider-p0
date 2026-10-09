package installdatabricks

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// The catalog names that the app's catalogNameError refuses, with its messages.
// An empty message means the name is valid.
func TestCatalogNameError(t *testing.T) {
	const (
		tooLong     = "Catalog names are at most 255 characters"
		forbidden   = "Catalog names can't contain a period, a space, a forward slash or a control character"
		urlPathOnly = `P0 can't install a catalog whose name contains #, ?, % or \. Contact support@p0.dev if you need P0 to manage it.`
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
		// Any whitespace that JavaScript's \s finds, as the app's check does.
		"sales\u00a0eu": forbidden,
		"sales\ufeffeu": forbidden,
		// \s doesn't find U+0085, and the app counts only ASCII control characters.
		"sales\u0085eu": "",
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

// The application IDs that the app's applicationId validator tests, with its message.
func TestApplicationIdValidator(t *testing.T) {
	cases := map[string]bool{
		"8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05": true,
		"":                                     false,
		"p0-connector":                         false,
		"8c5e2e0a8f0d4a3e9d613b2f4c7a1e05":     false,
	}

	for id, valid := range cases {
		resp := &validator.StringResponse{}
		uuidValidator("Application IDs").ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("application_id"),
			ConfigValue: types.StringValue(id),
		}, resp)
		errs := resp.Diagnostics.Errors()
		switch {
		case valid && len(errs) > 0:
			t.Errorf("uuidValidator rejects %q: %v", id, errs)
		case !valid && (len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Application IDs are UUIDs")):
			t.Errorf("uuidValidator(%q) = %v, want one error saying Application IDs are UUIDs", id, errs)
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

// A catalog's workspace_id comes from its key, which P0 sets the item's workspace
// from, so an item without a workspace reads the same.
func TestCatalogFromJsonTakesWorkspaceFromKey(t *testing.T) {
	var diags diag.Diagnostics
	got, ok := catalogFromJson(context.Background(), &diags, "main@1234567890123456", &catalogJson{}).(*catalogModel)
	if !ok || diags.HasError() {
		t.Fatalf("catalogFromJson returned %v, with %v", got, diags)
	}
	if got.WorkspaceId.ValueString() != "1234567890123456" || got.CatalogName.ValueString() != "main" {
		t.Errorf("catalogFromJson read workspace_id %s and catalog_name %s, want 1234567890123456 and main", got.WorkspaceId, got.CatalogName)
	}
}
