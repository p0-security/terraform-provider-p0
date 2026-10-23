package installlookerai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Decodes a state with every schema attribute into the model, as the framework
// does on plan and apply.
func decodeNullState(t *testing.T, r resource.Resource, model any) {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(objectType, values),
	}
	if diags := state.Get(ctx, model); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
}

func TestCredentialStagedModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewLookerAiCredentialStaged(), &lookerAiCredentialStagedModel{})
}

func TestCredentialModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewLookerAiCredential(), &lookerAiCredentialModel{})
}

// The wording names the instance and shows P0's message, without presuming that
// the connector is at fault.
func TestDescribeCheckError(t *testing.T) {
	const message = "422 Unprocessable Entity: Looker didn't accept the Admin key in p0_install_looker_acme_admin-key."
	summary, detail := describeCheckError("acme", errors.New(message))
	if summary != "Looker install check failed" {
		t.Errorf("summary = %q", summary)
	}
	if want := "P0 rejected the install check for the Looker instance \"acme\":\n\n" + message; detail != want {
		t.Errorf("detail =\n%s\nwant\n%s", detail, want)
	}
}

// The provider refuses at plan time what the P0 app refuses, with the app's rules.
func TestValidators(t *testing.T) {
	for _, c := range []struct {
		name  string
		match func(string) bool
		good  []string
		bad   []string
	}{
		{
			"id", itemIdRegex.MatchString,
			[]string{"acme", "Acme-Prod-2"},
			[]string{"", "acme_prod", "acme.prod", "acme/prod", "acme prod"},
		},
		{
			"instance url", instanceUrlRegex.MatchString,
			[]string{
				"https://acme.looker.app",
				"https://a-b.eu.looker.app",
				"https://a.looker.app",
			},
			[]string{
				"http://acme.looker.app",
				"https://acme.looker.app/",
				"https://ACME.Looker.App",
				"https://Acme.looker.app",
				"https://looker.app",
				"https://acme.looker.app/browse",
				"https://acme.looker.app:443",
				"https://acme.looker.app?x=1",
				"https://user@acme.looker.app",
				"https://-acme.looker.app",
				"https://acme-.looker.app",
				"https://acme..looker.app",
				"https://acme.looker.com",
				"https://acme.looker.app.evil.com",
				"https://acme.looker.app//",
				" https://acme.looker.app",
				"https://acme.looker.app\n",
			},
		},
		{
			"admin key secret", secretIdRegex.MatchString,
			[]string{"p0_install_looker_acme_admin-key", "a", strings.Repeat("a", 255)},
			[]string{"", "projects/p/secrets/key", "key.json", strings.Repeat("a", 255) + "a", `{"client_id":"x"}`},
		},
		{
			"content folder id", folderIdRegex.MatchString,
			[]string{"42", "abc_DEF-1", strings.Repeat("a", 64)},
			[]string{"", "/folders/42", "42/43", "4.2", "42 ", strings.Repeat("a", 65)},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, value := range c.good {
				if !c.match(value) {
					t.Errorf("%q is refused", value)
				}
			}
			for _, value := range c.bad {
				if c.match(value) {
					t.Errorf("%q is accepted", value)
				}
			}
		})
	}
}

// A value of the resource's schema with every attribute null except the strings in
// set.
func schemaValue(t *testing.T, r resource.Resource, set map[string]string) (tftypes.Value, resource.SchemaResponse) {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is %T, not an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
		if value, ok := set[name]; ok {
			values[name] = tftypes.NewValue(tftypes.String, value)
		}
	}
	return tftypes.NewValue(objectType, values), schemaResp
}

// Plans a change from prior to planned, where nil is no resource, and returns the
// diagnostics.
func modifyPlan(t *testing.T, prior, planned map[string]string) resource.ModifyPlanResponse {
	t.Helper()
	r := &LookerAiCredential{}
	nullValue, schemaResp := schemaValue(t, r, nil)
	value := func(set map[string]string) tftypes.Value {
		if set == nil {
			return tftypes.NewValue(nullValue.Type(), nil)
		}
		v, _ := schemaValue(t, r, set)
		return v
	}

	req := resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: value(prior)},
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: value(planned)},
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}
	r.ModifyPlan(context.Background(), req, &resp)
	return resp
}

// Only removing a folder that is set is refused, and the error points at the folder.
func TestModifyPlanRefusesFolderRemoval(t *testing.T) {
	folder := map[string]string{"id": "acme", "content_folder_id": "42"}
	noFolder := map[string]string{"id": "acme"}
	otherFolder := map[string]string{"id": "acme", "content_folder_id": "43"}

	for _, c := range []struct {
		name           string
		prior, planned map[string]string
	}{
		{"create", nil, noFolder},
		{"create with a folder", nil, folder},
		{"destroy", folder, nil},
		{"no folder", noFolder, noFolder},
		{"new folder", folder, otherFolder},
		{"first folder", noFolder, folder},
	} {
		t.Run(c.name, func(t *testing.T) {
			if resp := modifyPlan(t, c.prior, c.planned); resp.Diagnostics.HasError() {
				t.Errorf("refused: %v", resp.Diagnostics)
			}
		})
	}

	resp := modifyPlan(t, folder, noFolder)
	if resp.Diagnostics.ErrorsCount() != 1 {
		t.Fatalf("diagnostics = %v, want one error", resp.Diagnostics)
	}
	withPath, ok := resp.Diagnostics.Errors()[0].(diag.DiagnosticWithPath)
	if !ok || !withPath.Path().Equal(path.Root("content_folder_id")) {
		t.Errorf("error is not on content_folder_id: %v", resp.Diagnostics.Errors()[0])
	}
}
