package installdatabricksai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// A required string that P0 fixes once the item exists: the item's key, or a
// field marked `step: "new"` in the app's install schema, which P0 refuses to
// change after staging. RequiresReplace plans the destroy-and-create that works,
// in place of an update that P0 would refuse.
func fixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// Rejects a string with whitespace around it at plan time, with the message
// that p0_github_repositories gives. P0 trims the strings it stores, so the
// value would read back different from the configuration, which Terraform
// reports as an inconsistent result after apply.
func noWhitespaceAround() validator.String {
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
	value := req.ConfigValue.ValueString()
	if strings.TrimSpace(value) == value {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Whitespace around a value",
		fmt.Sprintf("P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. "+
			"Remove the whitespace, for example with trimspace().", req.Path),
	)
}
