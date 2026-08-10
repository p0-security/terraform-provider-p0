// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installagentic

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// definitionAttrTypes mirrors p0_agentic_server's flattened `definition`
// object, which discriminates on two sibling attributes at the same nesting
// level ("type": p0/custom, then "hosting": container/external) — the
// scenario RequiredWhenAttr/ExclusiveToAttr generalize
// access_policy.RequiredWhenType/ExclusiveToType to handle.
var definitionAttrTypes = map[string]attr.Type{
	"type":       types.StringType,
	"id":         types.StringType,
	"hosting":    types.StringType,
	"entrypoint": types.StringType,
	"image":      types.StringType,
}

func TestRequiredWhenAttr(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{
			name:    "p0 without id errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("p0"), "id": null, "hosting": null, "entrypoint": null, "image": null},
			wantErr: true,
		},
		{
			name:    "p0 with id passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("p0"), "id": set, "hosting": null, "entrypoint": null, "image": null},
			wantErr: false,
		},
		{
			name:    "unknown discriminator value is skipped",
			attrs:   map[string]attr.Value{"type": types.StringUnknown(), "id": null, "hosting": null, "entrypoint": null, "image": null},
			wantErr: false,
		},
		{
			name:    "container missing image errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("custom"), "id": null, "hosting": types.StringValue("container"), "entrypoint": set, "image": null},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(definitionAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenAttr("hosting", map[string][]string{
				"container": {"entrypoint", "image"},
			}).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("definition"), ConfigValue: object},
				resp,
			)
			// "hosting"-keyed validator only fires the container/image case;
			// exercise the "type"-keyed requirement separately below.
			if c.name == "p0 without id errors" || c.name == "p0 with id passes" {
				resp = &validator.ObjectResponse{}
				RequiredWhenAttr("type", map[string][]string{
					"p0": {"id"},
				}).ValidateObject(
					context.Background(),
					validator.ObjectRequest{Path: path.Root("definition"), ConfigValue: object},
					resp,
				)
			}
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestExclusiveToAttr(t *testing.T) {
	object, diags := types.ObjectValue(definitionAttrTypes, map[string]attr.Value{
		"type": types.StringValue("p0"), "id": types.StringValue("aws"),
		"hosting": types.StringNull(), "entrypoint": types.StringNull(), "image": types.StringValue("should-not-be-set"),
	})
	if diags.HasError() {
		t.Fatalf("failed to build object: %v", diags)
	}

	resp := &validator.ObjectResponse{}
	ExclusiveToAttr("type", map[string][]string{
		"custom": {"hosting", "image", "entrypoint"},
	}).ValidateObject(
		context.Background(),
		validator.ObjectRequest{Path: path.Root("definition"), ConfigValue: object},
		resp,
	)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error for 'image' set on a 'p0' definition, got none")
	}
}

// TestExclusiveToAttrSharedAttributeAcrossValues is a regression test: an
// attribute name (e.g. credential's "provider") listed under more than one
// discriminator value (e.g. both "aws" and "gcp") must not be rejected for
// either of those values — only for values that don't list it at all.
func TestExclusiveToAttrSharedAttributeAcrossValues(t *testing.T) {
	attrTypes := map[string]attr.Type{
		"type":     types.StringType,
		"provider": types.StringType,
		"grant":    types.StringType,
	}
	allowed := map[string][]string{
		"aws":   {"provider"},
		"gcp":   {"provider"},
		"oauth": {"grant"},
	}

	for _, discriminatorValue := range []string{"aws", "gcp"} {
		t.Run(discriminatorValue, func(t *testing.T) {
			object, diags := types.ObjectValue(attrTypes, map[string]attr.Value{
				"type":     types.StringValue(discriminatorValue),
				"provider": types.StringValue("some-id"),
				"grant":    types.StringNull(),
			})
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}

			resp := &validator.ObjectResponse{}
			ExclusiveToAttr("type", allowed).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("credential"), ConfigValue: object},
				resp,
			)

			if resp.Diagnostics.HasError() {
				t.Errorf("provider should be allowed when type is %q: %v", discriminatorValue, resp.Diagnostics)
			}
		})
	}

	t.Run("oauth rejects provider", func(t *testing.T) {
		object, diags := types.ObjectValue(attrTypes, map[string]attr.Value{
			"type":     types.StringValue("oauth"),
			"provider": types.StringValue("some-id"),
			"grant":    types.StringNull(),
		})
		if diags.HasError() {
			t.Fatalf("failed to build object: %v", diags)
		}

		resp := &validator.ObjectResponse{}
		ExclusiveToAttr("type", allowed).ValidateObject(
			context.Background(),
			validator.ObjectRequest{Path: path.Root("credential"), ConfigValue: object},
			resp,
		)

		if !resp.Diagnostics.HasError() {
			t.Errorf("provider should not be allowed when type is \"oauth\"")
		}
	})
}

func TestRequiredWhenAttrNullObject(t *testing.T) {
	resp := &validator.ObjectResponse{}
	RequiredWhenAttr("type", map[string][]string{"p0": {"id"}}).ValidateObject(
		context.Background(),
		validator.ObjectRequest{Path: path.Root("definition"), ConfigValue: types.ObjectNull(definitionAttrTypes)},
		resp,
	)
	if resp.Diagnostics.HasError() {
		t.Errorf("null object should not error: %v", resp.Diagnostics)
	}
}
