// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	installdatabricks "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/databricks"
)

// These tests cover the values that each Databricks validator refuses. They run
// the validators that a resource's schema attaches to an attribute, so they need
// no Terraform binary, and a case fails if its validator is detached. They live
// here, rather than beside the validators, because CI tests this package alone.

// The errors that the validators of a resource's string attribute give value,
// each as "<summary>: <detail>".
func databricksAttributeErrors(t *testing.T, newResource func() fwresource.Resource, attribute string, value string) []string {
	t.Helper()
	ctx := context.Background()

	var schemaResp fwresource.SchemaResponse
	newResource().Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	attr, ok := schemaResp.Schema.Attributes[attribute].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute", attribute)
	}

	var errs []string
	for _, v := range attr.Validators {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        path.Root(attribute),
			ConfigValue: types.StringValue(value),
		}, resp)
		for _, err := range resp.Diagnostics.Errors() {
			errs = append(errs, err.Summary()+": "+err.Detail())
		}
	}
	return errs
}

// Checks each value in cases against the attribute's validators. A case's
// message is part of the one error that its value gets, or "" if the validators
// accept the value.
func checkDatabricksAttribute(t *testing.T, newResource func() fwresource.Resource, attribute string, cases map[string]string) {
	t.Helper()
	for value, want := range cases {
		got := databricksAttributeErrors(t, newResource, attribute, value)
		switch {
		case want == "" && len(got) > 0:
			t.Errorf("%s refuses %q: %v", attribute, value, got)
		case want != "" && (len(got) != 1 || !strings.Contains(got[0], want)):
			t.Errorf("%s = %q gives %v, want one error with %q", attribute, value, got, want)
		}
	}
}

// The connector refuses a workspace ID that isn't all digits.
func TestDatabricksWorkspaceId(t *testing.T) {
	const numeric = "Databricks workspace IDs are numeric"
	checkDatabricksAttribute(t, installdatabricks.NewWorkspace, "id", map[string]string{
		"1234567890123456":  "",
		"7":                 "",
		"":                  numeric,
		"dbc-1234abcd-5678": numeric,
		"123 456":           numeric,
		"-1":                numeric,
	})
}

// The catalog names that the app's catalogNameError refuses, with its messages.
func TestDatabricksCatalogName(t *testing.T) {
	const (
		summary     = "Invalid catalog name: "
		tooLong     = summary + "Catalog names are at most 255 characters"
		forbidden   = summary + "Catalog names can't contain a period, a space, a forward slash or a control character"
		urlPathOnly = summary + `P0 can't install a catalog whose name contains #, ?, % or \. Contact support@p0.dev if you need P0 to manage it.`
	)
	checkDatabricksAttribute(t, installdatabricks.NewCatalog, "catalog_name", map[string]string{
		"main":                   "",
		"sales_prod":             "",
		"sales-prod":             "",
		"données":                "",
		strings.Repeat("a", 255): "",
		strings.Repeat("a", 256): tooLong,
		// JavaScript counts a character outside the BMP as two, and so does the app.
		strings.Repeat("😀", 127) + "a": "",
		strings.Repeat("😀", 128):       tooLong,
		"":                             summary + "Enter the catalog's name",
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
		"Sales":   summary + "Unity Catalog stores catalog names in lowercase, so enter sales",
		"DONNÉES": summary + "Unity Catalog stores catalog names in lowercase, so enter données",
		// P0's install API carries item keys in its URL paths unescaped.
		"sales#eu": urlPathOnly,
		"sales?eu": urlPathOnly,
		"a%62c":    urlPathOnly,
		`a\b`:      urlPathOnly,
	})
}

// The application IDs that the app's applicationId validator tests, with its
// message.
func TestDatabricksApplicationId(t *testing.T) {
	const uuid = "Application IDs are UUIDs"
	checkDatabricksAttribute(t, installdatabricks.NewAccount, "application_id", map[string]string{
		"8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05": "",
		"":                                     uuid,
		"p0-connector":                         uuid,
		"8c5e2e0a8f0d4a3e9d613b2f4c7a1e05":     uuid,
	})
}

// A commercial region passes. A value that isn't a region is told so, and only a
// region in another partition is told that the connector can't run there.
func TestDatabricksConnectorRegion(t *testing.T) {
	const (
		invalid     = "Invalid AWS region: Enter an AWS region, such as us-west-2"
		unsupported = "Unsupported AWS region: The connector runs only in commercial AWS regions"
	)
	checkDatabricksAttribute(t, installdatabricks.NewConnectorStaged, "region", map[string]string{
		"us-east-1":      "",
		"us-west-2":      "",
		"eu-central-2":   "",
		"ap-southeast-7": "",
		"ca-west-1":      "",
		"mx-central-1":   "",
		"il-central-1":   "",
		"us-gov-west-1":  unsupported,
		"us-gov-east-1":  unsupported,
		"cn-north-1":     unsupported,
		"us-isob-east-1": unsupported,
		"eusc-de-east-1": unsupported,
		"us-west":        invalid,
		" us-east-1":     invalid,
		"":               invalid,
	})
}

// P0 trims a connector's domain pattern, so one with whitespace around it would
// read back different from the configuration.
func TestDatabricksDomainPattern(t *testing.T) {
	const whitespace = "Whitespace around a value: P0 removes the whitespace around 'domain_pattern'"
	checkDatabricksAttribute(t, installdatabricks.NewConnectorStaged, "domain_pattern", map[string]string{
		`example\.com`:        "",
		`(.+\.)?example\.com`: "",
		// Whitespace inside a pattern is the pattern's own.
		`example\.com|example org`: "",
		` example\.com`:            whitespace,
		`example\.com `:            whitespace,
		"example\\.com\n":          whitespace,
		"":                         "string length must be at least 1",
	})
}

// Imports id into a null state, as the framework does, and returns the state
// and what the import reports.
func importDatabricks(t *testing.T, newResource func() fwresource.Resource, id string) (tfsdk.State, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()
	importer, ok := newResource().(fwresource.ResourceWithImportState)
	if !ok {
		t.Fatal("the resource doesn't import")
	}
	var schemaResp fwresource.SchemaResponse
	importer.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)

	resp := fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	importer.ImportState(ctx, fwresource.ImportStateRequest{ID: id}, &resp)
	return resp.State, resp.Diagnostics
}

// The imported state's string attribute.
func importedDatabricksAttribute(t *testing.T, state tfsdk.State, attribute string) string {
	t.Helper()
	var value types.String
	if diags := state.GetAttribute(context.Background(), path.Root(attribute), &value); diags.HasError() {
		t.Fatalf("could not read the imported %s: %v", attribute, diags)
	}
	return value.ValueString()
}

// A connector, an account and a workspace import by their item's key, which is
// their id, and refuse a key that their id would refuse. The read after an
// import puts the key in a URL path, where `%zz` doesn't parse and `#` ends the
// path.
func TestDatabricksImportId(t *testing.T) {
	const (
		awsAccountId = "AWS account IDs should consist of 12 numeric digits"
		accountId    = "Databricks account IDs are UUIDs"
		workspaceId  = "Databricks workspace IDs are numeric"
	)
	cases := []struct {
		name        string
		newResource func() fwresource.Resource
		id, refusal string
	}{
		{"ConnectorStaged", installdatabricks.NewConnectorStaged, "123456789012", awsAccountId},
		{"Connector", installdatabricks.NewConnector, "123456789012", awsAccountId},
		{"AccountStaged", installdatabricks.NewAccountStaged, "01234567-89ab-cdef-0123-456789abcdef", accountId},
		{"Account", installdatabricks.NewAccount, "01234567-89ab-cdef-0123-456789abcdef", accountId},
		{"Workspace", installdatabricks.NewWorkspace, "1234567890123456", workspaceId},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, diags := importDatabricks(t, c.newResource, c.id)
			if diags.HasError() {
				t.Fatalf("importing %q fails: %v", c.id, diags)
			}
			if got := importedDatabricksAttribute(t, state, "id"); got != c.id {
				t.Errorf("importing %q sets id %q", c.id, got)
			}

			for _, id := range []string{"12%zz", c.id + "#x", c.id + "?x", c.id + "/verify", ""} {
				_, diags := importDatabricks(t, c.newResource, id)
				if errs := diags.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Detail(), c.refusal) {
					t.Errorf("importing %q gives %v, want one error saying %q", id, errs, c.refusal)
				}
			}
		})
	}
}

// A catalog imports by its key, <catalog name>@<workspace ID>, which sets its
// catalog_name and workspace_id, and refuses an ID that isn't such a key.
func TestDatabricksCatalogImportKey(t *testing.T) {
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
		state, diags := importDatabricks(t, installdatabricks.NewCatalog, c.key)
		if !c.ok {
			if errs := diags.Errors(); len(errs) != 1 || errs[0].Summary() != "Bad import ID" {
				t.Errorf("importing %q gives %v, want one Bad import ID error", c.key, errs)
			}
			continue
		}
		if diags.HasError() {
			t.Errorf("importing %q fails: %v", c.key, diags)
			continue
		}

		id := importedDatabricksAttribute(t, state, "id")
		catalogName := importedDatabricksAttribute(t, state, "catalog_name")
		workspaceId := importedDatabricksAttribute(t, state, "workspace_id")
		if id != c.key || catalogName != c.catalogName || workspaceId != c.workspaceId {
			t.Errorf("importing %q sets id %q, catalog_name %q and workspace_id %q, want %q, %q and %q", c.key, id, catalogName, workspaceId, c.key, c.catalogName, c.workspaceId)
		}
	}
}
