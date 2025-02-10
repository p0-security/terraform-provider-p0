package installssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const azurePrefix = "azure:"

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &sshAzureIamWrite{}
var _ resource.ResourceWithConfigure = &sshAzureIamWrite{}
var _ resource.ResourceWithImportState = &sshAzureIamWrite{}

type sshAzureIamWrite struct {
	installer *common.Install
}

type sshAzureIamWriteModel struct {
	AdminAccessRoleId    types.String `tfsdk:"admin_access_role_id" json:"adminAccessRoleId,omitempty"`
	BastionId            types.String `tfsdk:"bastion_id" json:"bastionId,omitempty"`
	GroupKey             types.String `tfsdk:"group_key" json:"groupKey,omitempty"`
	IsSudoEnabled        types.Bool   `tfsdk:"is_sudo_enabled" json:"isSudoEnabled,omitempty"`
	Label                types.String `tfsdk:"label" json:"label,omitempty"`
	ManagementGroupId    types.String `tfsdk:"management_group_id" json:"managementGroupId,omitempty"`
	StandardAccessRoleId types.String `tfsdk:"standard_access_role_id" json:"standardAccessRoleId,omitempty"`
	State                types.String `tfsdk:"state" json:"state,omitempty"`
}

type sshAzureIamWriteJson struct {
	AdminAccessRoleId    *string `json:"adminAccessRoleId"`
	BastionId            *string `json:"bastionId"`
	GroupKey             *string `json:"groupKey"`
	IsSudoEnabled        *bool   `json:"isSudoEnabled,omitempty"`
	Label                *string `json:"label,omitempty"`
	ManagementGroupId    *string `json:"managementGroupId"`
	StandardAccessRoleId *string `json:"standardAccessRoleId"`
	State                string  `json:"state"`
}

type sshAzureIamWriteApi struct {
	Item *sshAzureIamWriteJson `json:"item"`
}

func NewSshAzureIamWrite() resource.Resource {
	return &sshAzureIamWrite{}
}

func (*sshAzureIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_ssh_azure"
}

func (*sshAzureIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Microsoft Azure SSH installation. 
		
Installing SSH allows you to manage access to your virtual machines on Microsoft Azure.`,
		Attributes: map[string]schema.Attribute{
			"admin_access_role_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the Azure role that grants admin access to the virtual machines`,
				Required:            true,
			},
			"bastion_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the Azure Bastion that provides secure RDP and SSH access to the virtual machines`,
				Required:            true,
			},
			"group_key": schema.StringAttribute{
				MarkdownDescription: `If present, virtual machines on Azure will be grouped by the value of this tag. Access can be requested, in one request, to all instances with a shared tag value`,
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"is_sudo_enabled": schema.BoolAttribute{
				MarkdownDescription: `If true, users will be able to request sudo access to the instances`,
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Azure Management Group label (if available)",
			},
			"management_group_id": schema.StringAttribute{
				MarkdownDescription: "The Azure Management Group ID",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"standard_access_role_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the Azure role that grants standard access to the virtual machines`,
				Required:            true,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
		},
	}
}

func (r *sshAzureIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  SshKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *sshAzureIamWrite) getId(data any) *string {
	model, ok := data.(*sshAzureIamWriteModel)
	if !ok {
		return nil
	}

	str := fmt.Sprintf("%s%s", azurePrefix, model.ManagementGroupId.ValueString())
	return &str
}

func (r *sshAzureIamWrite) getItemJson(json any) any {
	inner, ok := json.(*sshAzureIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *sshAzureIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := sshAzureIamWriteModel{}
	jsonv, ok := json.(*sshAzureIamWriteJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)
	data.ManagementGroupId = types.StringValue(strings.TrimPrefix(id, azurePrefix))

	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.GroupKey = types.StringNull()
	if jsonv.GroupKey != nil {
		data.GroupKey = types.StringValue(*jsonv.GroupKey)
	}

	data.IsSudoEnabled = types.BoolNull()
	if jsonv.IsSudoEnabled != nil {
		data.IsSudoEnabled = types.BoolValue(*jsonv.IsSudoEnabled)
	}

	data.StandardAccessRoleId = types.StringNull()
	if jsonv.StandardAccessRoleId != nil {
		data.StandardAccessRoleId = types.StringValue(*jsonv.StandardAccessRoleId)
	}

	data.AdminAccessRoleId = types.StringNull()
	if jsonv.AdminAccessRoleId != nil {
		data.AdminAccessRoleId = types.StringValue(*jsonv.AdminAccessRoleId)
	}
	data.BastionId = types.StringNull()
	if jsonv.BastionId != nil {
		data.BastionId = types.StringValue(*jsonv.BastionId)
	}

	return &data
}

func (r *sshAzureIamWrite) toJson(data any) any {
	json := sshAzureIamWriteJson{}

	datav, ok := data.(*sshAzureIamWriteModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	if !datav.GroupKey.IsNull() {
		group := datav.GroupKey.ValueString()
		json.GroupKey = &group
	}

	if !datav.IsSudoEnabled.IsNull() {
		isSudoEnabled := datav.IsSudoEnabled.ValueBool()
		json.IsSudoEnabled = &isSudoEnabled
	}

	if !datav.AdminAccessRoleId.IsNull() && !datav.AdminAccessRoleId.IsUnknown() {
		adminAccessRoleId := datav.AdminAccessRoleId.ValueString()
		json.AdminAccessRoleId = &adminAccessRoleId
	}

	if !datav.StandardAccessRoleId.IsNull() && !datav.StandardAccessRoleId.IsUnknown() {
		standardAccessRoleId := datav.StandardAccessRoleId.ValueString()
		json.StandardAccessRoleId = &standardAccessRoleId
	}

	if !datav.BastionId.IsNull() && !datav.BastionId.IsUnknown() {
		bastionId := datav.BastionId.ValueString()
		json.BastionId = &bastionId
	}

	// can omit state here as it's filled by the backend
	return &json
}

// Create implements resource.ResourceWithImportState.
func (s *sshAzureIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json sshAzureIamWriteApi
	var data sshAzureIamWriteModel
	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *sshAzureIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sshAzureIamWriteApi{}, &sshAzureIamWriteModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *sshAzureIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &sshAzureIamWriteModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *sshAzureIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &sshAzureIamWriteApi{}, &sshAzureIamWriteModel{})
}

func (s *sshAzureIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("management_group_id"), req, resp)
}
