package installauth0

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

// copy of internal/provider/resources/install/docker/vault_test.go, renamed for Auth0
// (INT-590).

func awsVault() *vaultModel {
	return &vaultModel{
		Type:          types.StringValue(awsSecretsManager),
		AccountId:     types.StringValue("123456789012"),
		ProjectId:     types.StringNull(),
		SecretsRegion: types.StringValue("us-west-2"),
	}
}

func gcpVault() *vaultModel {
	return &vaultModel{
		Type:          types.StringValue(gcpSecretManager),
		AccountId:     types.StringNull(),
		ProjectId:     types.StringValue("acme-secrets"),
		SecretsRegion: types.StringNull(),
	}
}

// The vault goes to P0 in the field names of the app's install schema, in which
// "install" names the AWS account or the Google Cloud project, and reads back as the
// model that was sent.
func TestVaultJson(t *testing.T) {
	cases := map[string]struct {
		model *vaultModel
		want  string
	}{
		"aws": {model: awsVault(), want: `{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"}`},
		"gcp": {model: gcpVault(), want: `{"type":"gcp-sm","install":"acme-secrets"}`},
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

			var stored vaultJson
			if err := json.Unmarshal(sent, &stored); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := vaultFromJson(&stored); !reflect.DeepEqual(got, c.model) {
				t.Errorf("vaultFromJson =\n%+v\nwant\n%+v", got, c.model)
			}
		})
	}
}

// Each field of the model is an attribute of the block, and the other way around.
func TestVaultFieldsMatchAttributes(t *testing.T) {
	attributes := vaultAttribute("The vault", "the secret").Attributes
	fields := awsVault().fields()
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

// The vault's type takes both secret managers.
func TestVaultTypeValidator(t *testing.T) {
	vaultType, ok := vaultAttribute("The vault", "the secret").Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("vault.type is not a string attribute")
	}
	for value, want := range map[string]bool{"aws-sm": true, "gcp-sm": true, "aws": false, "": false} {
		resp := &validator.StringResponse{}
		for _, v := range vaultType.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("type"), ConfigValue: types.StringValue(value)}, resp)
		}
		if got := !resp.Diagnostics.HasError(); got != want {
			t.Errorf("vault.type accepts %q = %v; want %v", value, got, want)
		}
	}
}

// A vault takes the fields of its own secret manager, and no other's. A value that
// Terraform doesn't know yet is neither missing nor unexpected.
func TestValidateVault(t *testing.T) {
	change := func(base *vaultModel, change func(*vaultModel)) *vaultModel {
		change(base)
		return base
	}

	cases := []struct {
		name  string
		vault *vaultModel
		// The summaries of the errors expected, in any order.
		want []string
	}{
		{name: "aws", vault: awsVault()},
		{name: "gcp", vault: gcpVault()},
		{name: "no vault", vault: nil},
		{name: "unknown type", vault: &vaultModel{Type: types.StringUnknown(), AccountId: types.StringNull(), ProjectId: types.StringNull(), SecretsRegion: types.StringNull()}},
		{name: "aws without an account", vault: change(awsVault(), func(v *vaultModel) { v.AccountId = types.StringNull() }), want: []string{"Missing AWS account ID"}},
		{name: "aws without a region", vault: change(awsVault(), func(v *vaultModel) { v.SecretsRegion = types.StringNull() }), want: []string{"Missing secrets region"}},
		{name: "aws with a project", vault: change(awsVault(), func(v *vaultModel) { v.ProjectId = types.StringValue("acme-secrets") }), want: []string{"Unexpected Google Cloud project"}},
		{name: "aws with an unknown account", vault: change(awsVault(), func(v *vaultModel) { v.AccountId = types.StringUnknown() })},
		{name: "aws with an unknown project", vault: change(awsVault(), func(v *vaultModel) { v.ProjectId = types.StringUnknown() })},
		{name: "gcp without a project", vault: change(gcpVault(), func(v *vaultModel) { v.ProjectId = types.StringNull() }), want: []string{"Missing Google Cloud project"}},
		{name: "gcp with an account", vault: change(gcpVault(), func(v *vaultModel) { v.AccountId = types.StringValue("123456789012") }), want: []string{"Unexpected AWS account ID"}},
		{name: "gcp with a region", vault: change(gcpVault(), func(v *vaultModel) { v.SecretsRegion = types.StringValue("us-west-2") }), want: []string{"Unexpected secrets region"}},
		{
			name: "gcp with an aws vault's fields",
			vault: change(awsVault(), func(v *vaultModel) {
				v.Type = types.StringValue(gcpSecretManager)
			}),
			want: []string{"Missing Google Cloud project", "Unexpected AWS account ID", "Unexpected secrets region"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateVault(c.vault, &diags)

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
