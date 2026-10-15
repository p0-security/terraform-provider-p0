package installlookerai

import (
	"context"
	"errors"
	"strings"
	"testing"

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
			[]string{"42", "abc_DEF-1"},
			[]string{"", "/folders/42", "42/43", "4.2", "42 "},
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
