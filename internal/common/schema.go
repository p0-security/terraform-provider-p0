package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Ends the description of a resource whose integration is in preview.
const NotePreview = `**Note:** This integration is currently in preview.`

// A required string that P0 fixes once the item exists: the item's key, or a field
// marked `step: "new"` in the app's install schema, which P0 refuses to change after
// staging. RequiresReplace plans the destroy-and-create that works, in place of an
// update that P0 would refuse.
func FixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// Rejects value, at attribute, if it has whitespace around it. P0 trims the strings it
// stores, so the value would read back different from the configuration, which
// Terraform reports as an inconsistent result after apply.
func RejectWhitespaceAround(attribute path.Path, value string, diags *diag.Diagnostics) {
	if strings.TrimSpace(value) == value {
		return
	}
	diags.AddAttributeError(
		attribute,
		"Whitespace around a value",
		fmt.Sprintf("P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. "+
			"Remove the whitespace, for example with trimspace().", attribute),
	)
}

// Rejects a string with whitespace around it at plan time, as RejectWhitespaceAround does.
func NoWhitespaceAround() validator.String {
	return noWhitespaceAroundValidator{}
}

type noWhitespaceAroundValidator struct{}

func (noWhitespaceAroundValidator) Description(context.Context) string {
	return "value must not have whitespace around it"
}

func (v noWhitespaceAroundValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (noWhitespaceAroundValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	RejectWhitespaceAround(req.Path, req.ConfigValue.ValueString(), &resp.Diagnostics)
}
