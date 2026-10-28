package installdynamodbai

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// copy of decodeNullState in internal/provider/resources/install/mongodb-atlas-ai/credential_test.go
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

func TestIamWriteModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewDynamoDbAiIamWrite(), &dynamoDbAiIamWriteModel{})
}

func TestIamWriteJsonRoundTrip(t *testing.T) {
	r := &dynamoDbAiIamWrite{}
	model := &dynamoDbAiIamWriteModel{
		Id:    types.StringValue("391052057035"),
		Label: types.StringValue("dev"),
		State: types.StringUnknown(),
	}

	json, ok := r.toJson(model).(*dynamoDbAiIamWriteJson)
	if !ok {
		t.Fatal("toJson did not return the item JSON")
	}
	json.State = "installed"

	var diags diag.Diagnostics
	got, ok := r.fromJson(context.Background(), &diags, "391052057035", json).(*dynamoDbAiIamWriteModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson: %v", diags)
	}
	if got.Id.ValueString() != "391052057035" || got.Label.ValueString() != "dev" || got.State.ValueString() != "installed" {
		t.Fatalf("round trip changed the item: %+v", got)
	}
}

func TestIamWriteRefusesAnAccountIdThatIsNotTwelveDigits(t *testing.T) {
	for _, id := range []string{"12345678901", "1234567890123", "abcdefghijkl"} {
		if awsAccountIdRegex.MatchString(id) {
			t.Errorf("accepted %q", id)
		}
	}
	if !awsAccountIdRegex.MatchString("391052057035") {
		t.Error("refused a 12-digit account ID")
	}
}
