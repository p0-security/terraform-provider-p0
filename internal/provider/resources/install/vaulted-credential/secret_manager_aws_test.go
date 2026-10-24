package installvaultedcredential

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSecretManagerWithAwsToJson(t *testing.T) {
	aws := SecretManagerWithAwsStagedModel{
		Type:          types.StringValue(AwsSecretsManager),
		AccountId:     types.StringValue("123456789012"),
		SecretsRegion: types.StringValue("us-west-2"),
		ProjectId:     types.StringNull(),
	}
	json := aws.ToJson()
	if json.Type != AwsSecretsManager || json.Install != "123456789012" || *json.SecretsRegion != "us-west-2" {
		t.Errorf("aws-sm ToJson = %+v", json)
	}

	gcp := SecretManagerWithAwsStagedModel{
		Type:      types.StringValue(GcpSecretManager),
		AccountId: types.StringNull(),
		ProjectId: types.StringValue("my-project"),
	}
	json = gcp.ToJson()
	if json.Type != GcpSecretManager || json.Install != "my-project" || json.SecretsRegion != nil {
		t.Errorf("gcp-sm ToJson = %+v", json)
	}
}

func TestSecretManagerWithAwsFromJson(t *testing.T) {
	region := "us-west-2"
	model := SecretManagerWithAwsFromJson(&SecretManagerWithAwsJson{
		SecretManagerJson: SecretManagerJson{Type: AwsSecretsManager, Install: "123456789012", ConnectorRegion: &region},
		SecretsRegion:     &region,
	})
	if model.AccountId.ValueString() != "123456789012" || model.SecretsRegion.ValueString() != region || !model.ProjectId.IsNull() {
		t.Errorf("aws-sm FromJson = %+v", model)
	}
	if !model.ConnectorServiceName.IsNull() || !model.ConnectorServiceUri.IsNull() {
		t.Errorf("aws-sm FromJson has Cloud Run fields: %+v", model)
	}

	model = SecretManagerWithAwsFromJson(&SecretManagerWithAwsJson{
		SecretManagerJson: SecretManagerJson{Type: GcpSecretManager, Install: "my-project"},
	})
	if model.ProjectId.ValueString() != "my-project" || !model.AccountId.IsNull() || !model.SecretsRegion.IsNull() {
		t.Errorf("gcp-sm FromJson = %+v", model)
	}
}

// A config whose secret_manager, with the given attributes, sets only the given fields.
func secretManagerConfig(t *testing.T, attributes map[string]schema.Attribute, fields map[string]string) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"secret_manager": schema.SingleNestedAttribute{
			Required:   true,
			Attributes: attributes,
		},
	}}
	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	smType, ok := objectType.AttributeTypes["secret_manager"].(tftypes.Object)
	if !ok {
		t.Fatal("secret_manager type is not an object")
	}
	values := map[string]tftypes.Value{}
	for name := range smType.AttributeTypes {
		if value, ok := fields[name]; ok {
			values[name] = tftypes.NewValue(tftypes.String, value)
		} else {
			values[name] = tftypes.NewValue(tftypes.String, nil)
		}
	}
	return tfsdk.Config{
		Schema: s,
		Raw: tftypes.NewValue(objectType, map[string]tftypes.Value{
			"secret_manager": tftypes.NewValue(smType, values),
		}),
	}
}

func TestValidateSecretManagerWithAws(t *testing.T) {
	for name, tc := range map[string]struct {
		fields map[string]string
		errors int
	}{
		"aws-sm with its fields": {map[string]string{"type": AwsSecretsManager, "account_id": "123456789012", "secrets_region": "us-west-2"}, 0},
		"gcp-sm with its field":  {map[string]string{"type": GcpSecretManager, "project_id": "my-project"}, 0},
		"aws-sm missing fields":  {map[string]string{"type": AwsSecretsManager}, 2},
		"gcp-sm missing project": {map[string]string{"type": GcpSecretManager}, 1},
		"aws-sm with project_id": {map[string]string{"type": AwsSecretsManager, "account_id": "123456789012", "secrets_region": "us-west-2", "project_id": "my-project"}, 1},
		"gcp-sm with aws fields": {map[string]string{"type": GcpSecretManager, "project_id": "my-project", "account_id": "123456789012", "secrets_region": "us-west-2"}, 2},
	} {
		// The final resource's secret_manager also has connector_service_uri.
		for resource, attributes := range map[string]map[string]schema.Attribute{
			"staged": SecretManagerWithAwsAttributes("the secret", ""),
			"final":  FinalSecretManagerWithAwsAttributes("the secret", ""),
		} {
			t.Run(resource+": "+name, func(t *testing.T) {
				var diags diag.Diagnostics
				ValidateSecretManagerWithAws(context.Background(), secretManagerConfig(t, attributes, tc.fields), &diags)
				if got := diags.ErrorsCount(); got != tc.errors {
					t.Errorf("errors = %d, want %d: %v", got, tc.errors, diags)
				}
			})
		}
	}
}
