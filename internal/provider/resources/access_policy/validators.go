// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"fmt"
	"strings"

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

// bundlePolicyPrefix is the name prefix of the policy rule each access bundle
// owns (`bundle:<bundleId>`), written by the P0 app's bundles API.
const bundlePolicyPrefix = "bundle:"

// notBundlePolicyName rejects policy names that start with `bundle:`. Policy
// names match case-insensitively, so a policy with that name would overwrite
// a bundle's rule, and destroying it would delete the rule.
type notBundlePolicyName struct{}

// NotBundlePolicyName returns a string validator rejecting names reserved for
// access bundles.
func NotBundlePolicyName() validator.String {
	return notBundlePolicyName{}
}

func (v notBundlePolicyName) Description(_ context.Context) string {
	return fmt.Sprintf("must not start with %q, which is reserved for access bundles", bundlePolicyPrefix)
}

func (v notBundlePolicyName) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v notBundlePolicyName) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	name := req.ConfigValue.ValueString()
	if !strings.HasPrefix(strings.ToLower(name), bundlePolicyPrefix) {
		return
	}

	bundleId := name[len(bundlePolicyPrefix):]
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Reserved policy name",
		fmt.Sprintf("Policy names starting with %q belong to access bundles; %q is the policy of bundle %q. "+
			"Manage the bundle with `p0_access_bundle` instead:\n\n"+
			"1. Replace this resource with a `removed` block so Terraform stops managing it without deleting the rule:\n\n"+
			"   removed {\n     from = <this resource's address>\n     lifecycle {\n       destroy = false\n     }\n   }\n\n"+
			"2. Import the bundle as `p0_access_bundle` with the ID %q.",
			bundlePolicyPrefix, name, bundleId, bundleId),
	)
}
