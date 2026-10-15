package installsalesforceai

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

func TestOrgStagedModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewSalesforceAiOrgStaged(), &salesforceAiOrgStagedModel{})
}

func TestOrgModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewSalesforceAiOrg(), &salesforceAiOrgModel{})
}

// P0 stores the agent app's consumer key as agentClientId.
func TestOrgSendsTheInstallerFieldNames(t *testing.T) {
	model := salesforceAiOrgModel{
		FullDomain:             types.StringValue("acme.my.salesforce.com"),
		ProvisionerConsumerKey: types.StringValue("provisioner-key"),
		AgentConsumerKey:       types.StringValue("agent-key"),
	}

	body, err := json.Marshal((&SalesforceAiOrg{}).toJson(&model))
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"fullDomain":             "acme.my.salesforce.com",
		"provisionerConsumerKey": "provisioner-key",
		"agentClientId":          "agent-key",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Errorf("%s = %v, want %v", key, fields[key], value)
		}
	}
}

func TestFullDomainPattern(t *testing.T) {
	for _, host := range []string{"acme.my.salesforce.com", "acme.develop.my.salesforce.com"} {
		if !fullDomainRegex.MatchString(host) {
			t.Errorf("%s should match", host)
		}
	}
	for _, host := range []string{"https://acme.my.salesforce.com", "acme.my.salesforce.com/", "evil.example.com"} {
		if fullDomainRegex.MatchString(host) {
			t.Errorf("%s should not match", host)
		}
	}
}
