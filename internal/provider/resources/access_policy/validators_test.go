// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requestorRequirements mirrors the type-conditional requirements enforced on
// the requestor object (see requestorAttribute).
var requestorRequirements = map[string][]string{
	"user":  {"uid"},
	"group": {"groups", "effect"},
}

var requestorAttrTypes = map[string]attr.Type{
	"type":   types.StringType,
	"uid":    types.StringType,
	"groups": types.StringType, // stand-in; the validator only checks null-ness
	"effect": types.StringType,
}

// TestRequiredWhenType verifies that attributes are required for the `type`
// values that list them, and ignored otherwise, mirroring the `required`
// arrays in the P0 app's shared/src/types/policy/types.json.
func TestRequiredWhenType(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{
			name:    "user without uid errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": null, "groups": null, "effect": null},
			wantErr: true,
		},
		{
			name:    "user with uid passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": set, "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "group missing effect errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": null},
			wantErr: true,
		},
		{
			name:    "group with groups and effect passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": set},
			wantErr: false,
		},
		{
			name:    "type without requirements passes despite null attrs",
			attrs:   map[string]attr.Value{"type": types.StringValue("any"), "uid": null, "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "unknown uid is allowed (deferred to apply)",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": types.StringUnknown(), "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "null type is skipped",
			attrs:   map[string]attr.Value{"type": null, "uid": null, "groups": null, "effect": null},
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(requestorAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenType(requestorRequirements).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestRequiredWhenTypeNullObject verifies a null object is skipped entirely.
func TestRequiredWhenTypeNullObject(t *testing.T) {
	resp := &validator.ObjectResponse{}
	RequiredWhenType(requestorRequirements).ValidateObject(
		context.Background(),
		validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: types.ObjectNull(requestorAttrTypes)},
		resp,
	)
	if resp.Diagnostics.HasError() {
		t.Errorf("null object should not error: %v", resp.Diagnostics)
	}
}
