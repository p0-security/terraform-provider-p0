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
var _ resource.ResourceWithUpgradeState = &sshAzureIamWrite{}

type sshAzureIamWrite struct {
	installer *common.Install
}

type sshAzureIamWriteModel struct {
	GroupKey       types.String `tfsdk:"group_key" json:"groupKey,omitempty"`
	IsSudoEnabled  types.Bool   `tfsdk:"is_sudo_enabled" json:"isSudoEnabled,omitempty"`
	Label          types.String `tfsdk:"label" json:"label,omitempty"`
	SubscriptionId types.String `tfsdk:"subscription_id" json:"subscriptionId,omitempty"`
	State          types.String `tfsdk:"state" json:"state,omitempty"`
}

type sshAzureIamWriteJson struct {
	GroupKey       *string `json:"groupKey"`
	IsSudoEnabled  *bool   `json:"isSudoEnabled,omitempty"`
	Label          *string `json:"label,omitempty"`
	SubscriptionId *string `json:"subscriptionId"`
	State          string  `json:"state"`
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
		// Version 1: the VM-access roles (`standard_access_role_id`, `admin_access_role_id`)
		// and `bastion_id` moved to the `p0_azure_bastion_host` component and were removed
		// from this resource (see UpgradeState).
		Version: 1,
		MarkdownDescription: `A Microsoft Azure SSH installation.

Installing SSH allows you to manage access to your virtual machines on Microsoft Azure.

The VM-access roles P0 assigns when access is requested, and the Azure Bastion host or jump host P0 connects through, are configured on the ` + "`p0_azure_bastion_host`" + ` component for the same subscription, not here.`,
		Attributes: map[string]schema.Attribute{
			"group_key": schema.StringAttribute{
				MarkdownDescription: `If present, virtual machines on Azure will be grouped by the value of this tag. Access can be requested, in one request, to all instances with a shared tag value`,
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"is_sudo_enabled": schema.BoolAttribute{
				MarkdownDescription: `If true, users will be able to request sudo access to the instances. Sudo access is granted with the admin role configured on the p0_azure_bastion_host component.`,
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Azure Subscription label (if available)",
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The Azure Subscription ID",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
		},
	}
}

// Schema version 0 additionally carried the VM-access roles and Bastion ID as required
// inputs; version 1 removed them (they now live on `p0_azure_bastion_host`). Only the
// retained fields are read from prior state.
type sshAzureIamWriteModelV0 struct {
	AdminAccessRoleId    types.String `tfsdk:"admin_access_role_id"`
	BastionId            types.String `tfsdk:"bastion_id"`
	GroupKey             types.String `tfsdk:"group_key"`
	IsSudoEnabled        types.Bool   `tfsdk:"is_sudo_enabled"`
	Label                types.String `tfsdk:"label"`
	SubscriptionId       types.String `tfsdk:"subscription_id"`
	StandardAccessRoleId types.String `tfsdk:"standard_access_role_id"`
	State                types.String `tfsdk:"state"`
}

func (r *sshAzureIamWrite) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"admin_access_role_id":    schema.StringAttribute{Required: true},
					"bastion_id":              schema.StringAttribute{Required: true},
					"group_key":               schema.StringAttribute{Optional: true, Computed: true},
					"is_sudo_enabled":         schema.BoolAttribute{Optional: true, Computed: true},
					"label":                   schema.StringAttribute{Computed: true},
					"subscription_id":         schema.StringAttribute{Required: true},
					"standard_access_role_id": schema.StringAttribute{Required: true},
					"state":                   schema.StringAttribute{Computed: true},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior sshAzureIamWriteModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				// Drop the role/bastion fields; they are configured on
				// p0_azure_bastion_host now.
				upgraded := sshAzureIamWriteModel{
					GroupKey:       prior.GroupKey,
					IsSudoEnabled:  prior.IsSudoEnabled,
					Label:          prior.Label,
					SubscriptionId: prior.SubscriptionId,
					State:          prior.State,
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, &upgraded)...)
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

	str := fmt.Sprintf("%s%s", azurePrefix, model.SubscriptionId.ValueString())
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
	data.SubscriptionId = types.StringValue(strings.TrimPrefix(id, azurePrefix))

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

	// can omit state here as it's filled by the backend
	return &json
}

// Create implements resource.ResourceWithImportState.
func (s *sshAzureIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json sshAzureIamWriteApi
	var data sshAzureIamWriteModel
	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
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
	resource.ImportStatePassthroughID(ctx, path.Root("subscription_id"), req, resp)
}
