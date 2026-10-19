package installgithubrepositories

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Whether attribute's validators accept value.
func accepts(t *testing.T, attribute schema.StringAttribute, value string) bool {
	t.Helper()
	resp := &validator.StringResponse{}
	for _, v := range attribute.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("type"), ConfigValue: types.StringValue(value)}, resp)
	}
	return !resp.Diagnostics.HasError()
}

// The vault's type takes only `aws-sm` for now.
func TestVaultTypeValidator(t *testing.T) {
	vaultType, ok := vaultAttribute().Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("vault.type is not a string attribute")
	}
	for value, want := range map[string]bool{"aws-sm": true, "gcp-sm": false, "": false} {
		if got := accepts(t, vaultType, value); got != want {
			t.Errorf("vault.type accepts %q = %v; want %v", value, got, want)
		}
	}
}
