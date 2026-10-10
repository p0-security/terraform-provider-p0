package installgithubrepositories

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestVaultValidators(t *testing.T) {
	ctx := context.Background()
	attribute := vaultAttribute()
	attrTypes := map[string]attr.Type{
		"type":           types.StringType,
		"account_id":     types.StringType,
		"project_id":     types.StringType,
		"secrets_region": types.StringType,
	}
	null := types.StringNull()
	set := func(value string) types.String { return types.StringValue(value) }

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{
			name:  "aws-sm",
			attrs: map[string]attr.Value{"type": set("aws-sm"), "account_id": set("123456789012"), "secrets_region": set("us-east-1"), "project_id": null},
		},
		{
			name:  "gcp-sm",
			attrs: map[string]attr.Value{"type": set("gcp-sm"), "account_id": null, "secrets_region": null, "project_id": set("my-project-id")},
		},
		{
			name:    "aws-sm without a region",
			attrs:   map[string]attr.Value{"type": set("aws-sm"), "account_id": set("123456789012"), "secrets_region": null, "project_id": null},
			wantErr: true,
		},
		{
			name:    "aws-sm without an account",
			attrs:   map[string]attr.Value{"type": set("aws-sm"), "account_id": null, "secrets_region": set("us-east-1"), "project_id": null},
			wantErr: true,
		},
		{
			name:    "aws-sm with a project",
			attrs:   map[string]attr.Value{"type": set("aws-sm"), "account_id": set("123456789012"), "secrets_region": set("us-east-1"), "project_id": set("my-project-id")},
			wantErr: true,
		},
		{
			name:    "gcp-sm without a project",
			attrs:   map[string]attr.Value{"type": set("gcp-sm"), "account_id": null, "secrets_region": null, "project_id": null},
			wantErr: true,
		},
		{
			name:    "gcp-sm with a region",
			attrs:   map[string]attr.Value{"type": set("gcp-sm"), "account_id": null, "secrets_region": set("us-east-1"), "project_id": set("my-project-id")},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(attrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			for _, v := range attribute.Validators {
				v.ValidateObject(ctx, validator.ObjectRequest{Path: path.Root("vault"), ConfigValue: object}, resp)
			}
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}
