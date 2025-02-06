package installazure

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

const (
	AzureKey = "azure"
)

var managementGroupIdAttribute = schema.StringAttribute{
	Description: "The ID of the Azure Management Group.",
	Required:    true,
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var labelAttribute = schema.StringAttribute{
	Description: "The label of the Azure Management Group.",
	Computed:    true,
}
