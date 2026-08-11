// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installagentic

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requiredWhenAttr and exclusiveToAttr are generalizations of the
// RequiredWhenType/ExclusiveToType validators in
// internal/provider/resources/access_policy/validators.go: `p0_agentic_server`
// discriminates its `definition` object on two sibling attributes at the same
// nesting level (`type`: p0/custom, then `hosting`: container/external), so a
// single discriminator hardcoded to "type" isn't enough. These take the
// discriminator attribute's name as a parameter instead.

type requiredWhenAttr struct {
	discriminator string
	requirements  map[string][]string
}

// RequiredWhenAttr returns an object validator enforcing that, for each value
// of the named discriminator attribute, the listed sibling attributes are
// present and non-null.
func RequiredWhenAttr(discriminator string, requirements map[string][]string) validator.Object {
	return requiredWhenAttr{discriminator: discriminator, requirements: requirements}
}

func (v requiredWhenAttr) Description(_ context.Context) string {
	return fmt.Sprintf("some attributes are required depending on the value of `%s`", v.discriminator)
}

func (v requiredWhenAttr) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v requiredWhenAttr) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()

	discriminatorValue, ok := attributes[v.discriminator].(types.String)
	if !ok || discriminatorValue.IsNull() || discriminatorValue.IsUnknown() {
		return
	}

	required, ok := v.requirements[discriminatorValue.ValueString()]
	if !ok {
		return
	}

	for _, name := range required {
		value, present := attributes[name]
		if !present || value.IsNull() {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtName(name),
				"Missing required attribute",
				fmt.Sprintf("`%s` is required when `%s` is %q.", name, v.discriminator, discriminatorValue.ValueString()),
			)
		}
	}
}

type exclusiveToAttr struct {
	discriminator string
	allowed       map[string][]string
}

// ExclusiveToAttr returns an object validator enforcing that each listed
// attribute is null unless the named discriminator attribute equals the
// value it's scoped to — the inverse of RequiredWhenAttr.
func ExclusiveToAttr(discriminator string, allowed map[string][]string) validator.Object {
	return exclusiveToAttr{discriminator: discriminator, allowed: allowed}
}

func (v exclusiveToAttr) Description(_ context.Context) string {
	return fmt.Sprintf("some attributes may only be set for specific values of `%s`", v.discriminator)
}

func (v exclusiveToAttr) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v exclusiveToAttr) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()

	discriminatorValue, ok := attributes[v.discriminator].(types.String)
	if !ok || discriminatorValue.IsNull() || discriminatorValue.IsUnknown() {
		return
	}

	// Invert to attribute -> set of discriminator values it's permitted under,
	// since the same attribute (e.g. "provider") may legitimately be shared by
	// more than one discriminator value (e.g. both "aws" and "gcp"). Iterating
	// per discriminator value directly, as RequiredWhenAttr does, would flag
	// that attribute as "not allowed" as soon as any *other* value's list is
	// checked, even though the current value also permits it.
	permittedValues := map[string]map[string]bool{}
	for allowedValue, names := range v.allowed {
		for _, name := range names {
			if permittedValues[name] == nil {
				permittedValues[name] = map[string]bool{}
			}
			permittedValues[name][allowedValue] = true
		}
	}

	for name, allowed := range permittedValues {
		if allowed[discriminatorValue.ValueString()] {
			continue
		}
		value, present := attributes[name]
		if present && !value.IsNull() && !value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtName(name),
				"Attribute not allowed",
				fmt.Sprintf("`%s` is not valid when `%s` is %q.", name, v.discriminator, discriminatorValue.ValueString()),
			)
		}
	}
}
