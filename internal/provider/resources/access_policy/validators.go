// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requiredWhenType validates that the sibling attributes the P0 API requires
// for a given `type` discriminator value are actually set. Terraform's schema
// can only mark an attribute globally Required or Optional, but the requestor,
// resource, and approval objects are discriminated unions where an attribute is
// required only for some `type` values (e.g. `uid` only when `type` is "user").
// This mirrors the `required` arrays in the P0 app's
// shared/src/types/policy/types.json so plans fail fast instead of the backend
// rejecting the request.
type requiredWhenType struct {
	// requirements maps a `type` value to the attribute names required for it.
	requirements map[string][]string
}

// RequiredWhenType returns an object validator enforcing that, for each `type`
// value, the listed sibling attributes are present and non-null.
func RequiredWhenType(requirements map[string][]string) validator.Object {
	return requiredWhenType{requirements: requirements}
}

func (v requiredWhenType) Description(_ context.Context) string {
	return "some attributes are required depending on the value of `type`"
}

func (v requiredWhenType) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v requiredWhenType) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	// Skip when the whole object, or the discriminator, isn't known yet.
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()

	typeValue, ok := attributes["type"].(types.String)
	if !ok || typeValue.IsNull() || typeValue.IsUnknown() {
		return
	}

	required, ok := v.requirements[typeValue.ValueString()]
	if !ok {
		return
	}

	for _, name := range required {
		// A missing attribute (null) is an error; an unknown value (e.g. a
		// reference to another resource) can't be validated yet, so allow it.
		value, present := attributes[name]
		if !present || value.IsNull() {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtName(name),
				"Missing required attribute",
				fmt.Sprintf("`%s` is required when `type` is %q.", name, typeValue.ValueString()),
			)
		}
	}
}

// exclusiveToType validates that the listed sibling attributes are only set
// when `type` equals the given value — the inverse of requiredWhenType.
// Without this, an attribute belonging to one variant of a discriminated
// union could be set alongside an unrelated `type` value and reach apply;
// conversion code that special-cases one variant (e.g. agentToJson, which
// only forwards `groups`/`effect` for the "owner-group" agent type) would
// then silently drop it instead of the plan failing fast.
type exclusiveToType struct {
	// allowed maps a `type` value to the attribute names that may only be
	// set when `type` is that value.
	allowed map[string][]string
}

// ExclusiveToType returns an object validator enforcing that each listed
// attribute is null unless `type` equals the value it's scoped to.
func ExclusiveToType(allowed map[string][]string) validator.Object {
	return exclusiveToType{allowed: allowed}
}

func (v exclusiveToType) Description(_ context.Context) string {
	return "some attributes may only be set for specific values of `type`"
}

func (v exclusiveToType) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v exclusiveToType) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	// Skip when the whole object, or the discriminator, isn't known yet.
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()

	typeValue, ok := attributes["type"].(types.String)
	if !ok || typeValue.IsNull() || typeValue.IsUnknown() {
		return
	}

	for allowedType, names := range v.allowed {
		if typeValue.ValueString() == allowedType {
			continue
		}
		for _, name := range names {
			// An unset (null) attribute is fine; an unknown value can't be
			// validated yet, so allow it through to apply time.
			value, present := attributes[name]
			if present && !value.IsNull() && !value.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					req.Path.AtName(name),
					"Attribute not allowed",
					fmt.Sprintf("`%s` may only be used when `type` is %q; it is not valid when `type` is %q.", name, allowedType, typeValue.ValueString()),
				)
			}
		}
	}
}
