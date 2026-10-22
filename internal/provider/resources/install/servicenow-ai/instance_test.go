package installservicenowai

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

func TestInstanceStagedModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewServiceNowAiInstanceStaged(), &serviceNowAiInstanceStagedModel{})
}

func TestInstanceModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewServiceNowAiInstance(), &serviceNowAiInstanceModel{})
}

func TestInstanceSendsTheInstallerFieldNames(t *testing.T) {
	model := serviceNowAiInstanceModel{
		ProvisionerClientId: types.StringValue("provisioner-client"),
		JwtClientId:         types.StringValue("jwt-client"),
		KeyId:               types.StringValue("p0-signing"),
	}

	body, err := json.Marshal((&ServiceNowAiInstance{}).toJson(&model))
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"provisionerClientId": "provisioner-client",
		"jwtClientId":         "jwt-client",
		"keyId":               "p0-signing",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Errorf("%s = %v, want %v", key, fields[key], value)
		}
	}
}

func TestInstanceIdAcceptsOnlyInstanceNames(t *testing.T) {
	for id, want := range map[string]bool{
		"dev221998":            true,
		"acme-prod":            true,
		"Acme":                 false,
		"acme.service-now.com": false,
		"acme_prod":            false,
		"https://acme":         false,
	} {
		if got := instanceIdRegex.MatchString(id); got != want {
			t.Errorf("%q: match = %v, want %v", id, got, want)
		}
	}
}
