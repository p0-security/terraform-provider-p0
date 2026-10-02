package common

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// GetKnownObject reads the object attribute at p of a configuration into target, a
// pointer to a struct with tfsdk tags, and reports whether it did. It skips a null
// object, and an object that Terraform only knows at apply time, such as one set from
// a module output or a variable while validating: reading an unknown object straight
// into a struct fails with a conversion error instead. The object's own attributes
// may still be unknown.
func GetKnownObject(ctx context.Context, config tfsdk.Config, p path.Path, target any, diags *diag.Diagnostics) bool {
	var object types.Object
	getDiags := config.GetAttribute(ctx, p, &object)
	diags.Append(getDiags...)
	if getDiags.HasError() || object.IsNull() || object.IsUnknown() {
		return false
	}

	asDiags := object.As(ctx, target, basetypes.ObjectAsOptions{})
	diags.Append(asDiags...)
	return !asDiags.HasError()
}
