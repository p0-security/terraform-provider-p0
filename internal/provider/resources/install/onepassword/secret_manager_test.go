package installonepassword

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func awsSM() *secretManagerModel {
	return &secretManagerModel{
		Type:          types.StringValue(awsSecretsManager),
		AccountId:     types.StringValue("123456789012"),
		ProjectId:     types.StringNull(),
		SecretsRegion: types.StringValue("us-west-2"),
	}
}

func gcpSM() *secretManagerModel {
	return &secretManagerModel{
		Type:          types.StringValue(gcpSecretManager),
		AccountId:     types.StringNull(),
		ProjectId:     types.StringValue("acme-secrets"),
		SecretsRegion: types.StringNull(),
	}
}

// The secret manager goes to P0 in the field names of the app's install schema, in which
// "install" names the AWS account or the Google Cloud project, and reads back as the
// model that was sent.
func TestSecretManagerJson(t *testing.T) {
	cases := map[string]struct {
		model *secretManagerModel
		want  string
	}{
		"aws": {model: awsSM(), want: `{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"}`},
		"gcp": {model: gcpSM(), want: `{"type":"gcp-sm","install":"acme-secrets"}`},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			sent, err := json.Marshal(c.model.toJson())
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(sent) != c.want {
				t.Errorf("toJson =\n%s\nwant\n%s", sent, c.want)
			}

			var stored secretManagerJson
			if err := json.Unmarshal(sent, &stored); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := secretManagerFromJson(&stored); !reflect.DeepEqual(got, c.model) {
				t.Errorf("secretManagerFromJson =\n%+v\nwant\n%+v", got, c.model)
			}
		})
	}
}

// Each field of the model is an attribute of the block, and the other way around.
func TestSecretManagerFieldsMatchAttributes(t *testing.T) {
	attributes := secretManagerAttribute("The secret manager", "the secret").Attributes
	fields := awsSM().fields()
	for name := range fields {
		if _, ok := attributes[name]; !ok {
			t.Errorf("the model has %q, which the attribute doesn't", name)
		}
	}
	for name := range attributes {
		if _, ok := fields[name]; !ok && name != "type" {
			t.Errorf("the attribute has %q, which the model doesn't", name)
		}
	}
}

// The secret manager's type takes both secret managers.
func TestSecretManagerTypeValidator(t *testing.T) {
	secretManagerType, ok := secretManagerAttribute("The secret manager", "the secret").Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("secret_manager.type is not a string attribute")
	}
	for value, want := range map[string]bool{"aws-sm": true, "gcp-sm": true, "aws": false, "": false} {
		resp := &validator.StringResponse{}
		for _, v := range secretManagerType.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("type"), ConfigValue: types.StringValue(value)}, resp)
		}
		if got := !resp.Diagnostics.HasError(); got != want {
			t.Errorf("secret_manager.type accepts %q = %v; want %v", value, got, want)
		}
	}
}

// A secret manager takes the fields of its own secret manager, and no other's. A value that
// Terraform doesn't know yet is neither missing nor unexpected.
func TestValidateSecretManager(t *testing.T) {
	change := func(base *secretManagerModel, change func(*secretManagerModel)) *secretManagerModel {
		change(base)
		return base
	}

	cases := []struct {
		name          string
		secretManager *secretManagerModel
		// The summaries of the errors expected, in any order.
		want []string
	}{
		{name: "aws", secretManager: awsSM()},
		{name: "gcp", secretManager: gcpSM()},
		{name: "no secret manager", secretManager: nil},
		{name: "unknown type", secretManager: &secretManagerModel{Type: types.StringUnknown(), AccountId: types.StringNull(), ProjectId: types.StringNull(), SecretsRegion: types.StringNull()}},
		{name: "aws without an account", secretManager: change(awsSM(), func(v *secretManagerModel) { v.AccountId = types.StringNull() }), want: []string{"Missing AWS account ID"}},
		{name: "aws without a region", secretManager: change(awsSM(), func(v *secretManagerModel) { v.SecretsRegion = types.StringNull() }), want: []string{"Missing secrets region"}},
		{name: "aws with a project", secretManager: change(awsSM(), func(v *secretManagerModel) { v.ProjectId = types.StringValue("acme-secrets") }), want: []string{"Unexpected Google Cloud project"}},
		{name: "aws with an unknown account", secretManager: change(awsSM(), func(v *secretManagerModel) { v.AccountId = types.StringUnknown() })},
		{name: "aws with an unknown project", secretManager: change(awsSM(), func(v *secretManagerModel) { v.ProjectId = types.StringUnknown() })},
		{name: "gcp without a project", secretManager: change(gcpSM(), func(v *secretManagerModel) { v.ProjectId = types.StringNull() }), want: []string{"Missing Google Cloud project"}},
		{name: "gcp with an account", secretManager: change(gcpSM(), func(v *secretManagerModel) { v.AccountId = types.StringValue("123456789012") }), want: []string{"Unexpected AWS account ID"}},
		{name: "gcp with a region", secretManager: change(gcpSM(), func(v *secretManagerModel) { v.SecretsRegion = types.StringValue("us-west-2") }), want: []string{"Unexpected secrets region"}},
		{
			name: "gcp with an aws secret manager's fields",
			secretManager: change(awsSM(), func(v *secretManagerModel) {
				v.Type = types.StringValue(gcpSecretManager)
			}),
			want: []string{"Missing Google Cloud project", "Unexpected AWS account ID", "Unexpected secrets region"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateSecretManager(c.secretManager, &diags)

			got := []string{}
			for _, d := range diags.Errors() {
				got = append(got, d.Summary())
			}
			want := append([]string{}, c.want...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors = %v; want %v", got, want)
			}
		})
	}
}
