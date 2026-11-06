package installgitguardianai

import (
	"context"
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
	decodeNullState(t, NewGitguardianAiCredentialStaged(), &gitguardianAiCredentialStagedModel{})
}

func TestCredentialModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewGitguardianAiCredential(), &gitguardianAiCredentialModel{})
}

func TestWorkspaceIdIsDigitsOnly(t *testing.T) {
	for _, id := range []string{"942194", "1"} {
		if !itemIdRegex.MatchString(id) {
			t.Errorf("refused %q", id)
		}
	}
	for _, id := range []string{"", "abc", "94-21", "942194 "} {
		if itemIdRegex.MatchString(id) {
			t.Errorf("accepted %q", id)
		}
	}
}

func TestRegionRoundTripsThroughJson(t *testing.T) {
	json := regionToJson(types.StringValue("eu"))
	if json == nil || json.Type != "eu" {
		t.Fatalf("toJson: %+v", json)
	}
	if got := regionFromJson(json); got.ValueString() != "eu" {
		t.Fatalf("fromJson: %v", got)
	}
	if regionToJson(types.StringNull()) != nil {
		t.Fatal("a null region should send no region")
	}
}
