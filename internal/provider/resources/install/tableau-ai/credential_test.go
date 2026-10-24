package installtableauai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	decodeNullState(t, NewTableauAiCredentialStaged(), &tableauAiCredentialStagedModel{})
}

func TestCredentialModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewTableauAiCredential(), &tableauAiCredentialModel{})
}

func TestCredentialSendsTheInstallerFieldNames(t *testing.T) {
	model := tableauAiCredentialModel{
		Server:       types.StringValue("https://us-east-1.online.tableau.com"),
		ContentUrl:   types.StringValue("acme"),
		AdminUser:    types.StringValue("admin@example.com"),
		AgentMailbox: types.StringValue("agents@example.com"),
	}

	body, err := json.Marshal((&TableauAiCredential{}).toJson(&model))
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"server":       "https://us-east-1.online.tableau.com",
		"contentUrl":   "acme",
		"adminUser":    "admin@example.com",
		"agentMailbox": "agents@example.com",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Errorf("%s = %v, want %v", key, fields[key], value)
		}
	}
}

func TestServerAcceptsOnlyPodUrls(t *testing.T) {
	for server, want := range map[string]bool{
		"https://us-east-1.online.tableau.com":             true,
		"https://prod-useast-a.online.tableau.com":         true,
		"https://10ay.online.tableau.com":                  true,
		"https://us-east-1.online.tableau.com/":            false,
		"http://us-east-1.online.tableau.com":              false,
		"us-east-1.online.tableau.com":                     false,
		"https://online.tableau.com":                       false,
		"https://tableau.example.com":                      false,
		"https://us-east-1.online.tableau.com/#/site/acme": false,
	} {
		if got := serverRegex.MatchString(server); got != want {
			t.Errorf("%q: match = %v, want %v", server, got, want)
		}
	}
}
