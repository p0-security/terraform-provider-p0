package installgcp

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

type gcpRoleMetadata struct {
	Id   string `json:"id" tfsdk:"id"`
	Name string `json:"name" tfsdk:"name"`
}

type gcpPermissionsMetadata struct {
	Permissions []string        `json:"requiredPermissions" tfsdk:"permissions"`
	Role        gcpRoleMetadata `json:"customRole" tfsdk:"custom_role"`
}

type gcpPermissionsMetadataWithPredefinedRole struct {
	PredefinedRole string          `json:"predefinedRole" tfsdk:"predefined_role"`
	Permissions    []string        `json:"requiredPermissions" tfsdk:"permissions"`
	CustomRole     gcpRoleMetadata `json:"customRole" tfsdk:"custom_role"`
}

type gcpItemModel struct {
	Project string       `tfsdk:"project"`
	State   types.String `tfsdk:"state"`
}

type gcpItemJson struct {
	State string `json:"state"`
}

type gcpItemApi struct {
	Item gcpItemJson `json:"item"`
}

const (
	AccessLogs         = "access-logs"
	GcpKey             = "gcloud"
	SharingRestriction = "sharing-restriction"
)

var GcpProjectIdRegex = regexp.MustCompile(`^[\w-]+$`)
var GcpOrganizationIdRegex = regexp.MustCompile(`^[\d]+$`)

var customRole = schema.SingleNestedAttribute{
	Computed:            true,
	MarkdownDescription: `Describes the custom role that should be created and assigned to P0's service account`,
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: `The custom role expected identifier`,
		},
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: `The custom role's expected title`,
		},
	},
}

var predefinedRole = schema.StringAttribute{
	Computed:            true,
	MarkdownDescription: `The predefined role that should be granted to P0, in order to install projects for IAM management`,
}

var itemAttributes = map[string]schema.Attribute{
	// In P0 we would name this 'id' or 'project_id'; it is named 'project' here to align with Terraform's naming for
	// Google Cloud resources
	"project": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The ID of the Google Cloud project to manage with P0",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"state": schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: installresources.StateMarkdownDescription,
	},
}

func permissions(name string) schema.ListAttribute {
	return schema.ListAttribute{
		ElementType: types.StringType,
		Computed:    true,
		MarkdownDescription: `Permissions that should be granted to P0 via the custom role, described in the 'role' attribute,
in order to install projects for ` + name,
	}
}

func itemGetId(data any) *string {
	model, ok := data.(*gcpItemModel)
	if !ok {
		return nil
	}
	return &model.Project
}

func itemGetItemJson(json any) any {
	inner, ok := json.(*gcpItemApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func itemFromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpItemModel{}
	jsonv, ok := json.(*gcpItemJson)
	if !ok {
		return nil
	}

	data.Project = id
	data.State = types.StringValue(jsonv.State)

	return &data
}

func itemToJson(data any) any {
	json := gcpItemJson{}

	// can omit state here as it's filled by the backend
	return json
}

func newItemInstaller(component string, providerData *internal.P0ProviderData) *installresources.Install {
	return &installresources.Install{
		Integration:  GcpKey,
		Component:    component,
		ProviderData: providerData,
		GetId:        itemGetId,
		GetItemJson:  itemGetItemJson,
		FromJson:     itemFromJson,
		ToJson:       itemToJson,
	}
}