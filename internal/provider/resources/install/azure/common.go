package installazure

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

const (
	AzureKey    = "azure"
	AzureAppKey = "azure-app"
)

var subscriptionIdAttribute = schema.StringAttribute{
	Description: "The ID of the Azure Subscription.",
	Required:    true,
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var labelAttribute = schema.StringAttribute{
	Description: "The label of the Azure Subscription.",
	Computed:    true,
}

func singletonGetId(data any) *string {
	key := common.SingletonKey
	return &key
}
