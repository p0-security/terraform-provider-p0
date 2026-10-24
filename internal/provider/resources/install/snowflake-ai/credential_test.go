package installsnowflakeai

import (
	"context"
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
	decodeNullState(t, NewSnowflakeAiCredentialStaged(), &snowflakeAiCredentialStagedModel{})
}

func TestCredentialModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewSnowflakeAiCredential(), &snowflakeAiCredentialModel{})
}

func TestItemIdRegex(t *testing.T) {
	for id, want := range map[string]bool{
		"MYORG-MYACCOUNT":     true,
		"MYORG-MY_ACCOUNT":    true,
		"ORG1-ACCOUNT_2_PROD": true,
		"myorg-myaccount":     false,
		"MYORG":               false,
		"MYORG-MY-ACCOUNT":    false,
		"MY_ORG-MYACCOUNT":    false,
		"-MYACCOUNT":          false,
		"MYORG-":              false,
		"XY12345.US-EAST-1":   false,
		"MYORG-MYACCOUNT/X":   false,
		" MYORG-MYACCOUNT":    false,
	} {
		if got := itemIdRegex.MatchString(id); got != want {
			t.Errorf("itemIdRegex.MatchString(%q) = %v, want %v", id, got, want)
		}
	}
}
