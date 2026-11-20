package installawsssmai

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// A copy of the Redshift (AI) install resources' schema.go.

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

// A copy of adminSwitchAttribute in the Databricks install resources'
// common.go.
//
// A configure-step switch: off unless the configuration turns it on, and
// changed in place. The provider always sends it, so removing it from a
// configuration turns it off in P0.
func switchAttribute(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(false),
		MarkdownDescription: description,
	}
}
