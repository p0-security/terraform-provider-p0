package entra_id

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var tenantIdAttribute = schema.StringAttribute{
	Description: "The Entra ID (Azure AD) tenant ID.",
	Required:    true,
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var labelAttribute = schema.StringAttribute{
	Description: "The label of the tenant (computed from P0).",
	Computed:    true,
}

var stateAttribute = schema.StringAttribute{
	Computed:            true,
	MarkdownDescription: common.StateMarkdownDescription,
}
