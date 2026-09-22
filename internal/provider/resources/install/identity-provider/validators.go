// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installidentityprovider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requiredWhenAttr and exclusiveToAttr validate `replay_protection`'s `type`
// discriminator against its sibling attributes, mirroring
// internal/provider/resources/install/agentic/validators.go.

type requiredWhenAttr struct {
	requirements map[string][]string
}

// RequiredWhenAttr returns an object validator enforcing that, for each value
// of the `type` attribute, the listed sibling attributes are present and
// non-null.
func RequiredWhenAttr(requirements map[string][]string) validator.Object {
	return requiredWhenAttr{requirements: requirements}
}

func (v requiredWhenAttr) Description(_ context.Context) string {
	return "some attributes are required depending on the value of `type`"
}

func (v requiredWhenAttr) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v requiredWhenAttr) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
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

type exclusiveToAttr struct {
	allowed map[string][]string
}

// ExclusiveToAttr returns an object validator enforcing that each listed
// attribute is null unless `type` equals the value it's scoped to — the
// inverse of RequiredWhenAttr.
func ExclusiveToAttr(allowed map[string][]string) validator.Object {
	return exclusiveToAttr{allowed: allowed}
}

func (v exclusiveToAttr) Description(_ context.Context) string {
	return "some attributes may only be set for specific values of `type`"
}

func (v exclusiveToAttr) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v exclusiveToAttr) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()

	typeValue, ok := attributes["type"].(types.String)
	if !ok || typeValue.IsNull() || typeValue.IsUnknown() {
		return
	}

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
		if allowed[typeValue.ValueString()] {
			continue
		}
		value, present := attributes[name]
		if present && !value.IsNull() && !value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtName(name),
				"Attribute not allowed",
				fmt.Sprintf("`%s` is not valid when `type` is %q.", name, typeValue.ValueString()),
			)
		}
	}
}
