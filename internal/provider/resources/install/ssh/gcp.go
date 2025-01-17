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

const gcloudPrefix = "gcloud:"

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &sshGcpIamWrite{}
var _ resource.ResourceWithConfigure = &sshGcpIamWrite{}
var _ resource.ResourceWithImportState = &sshGcpIamWrite{}

type sshGcpIamWrite struct {
	installer *common.Install
}

type sshGcpIamWriteModel struct {
	GroupKey      types.String `tfsdk:"group_key" json:"groupKey,omitempty"`
	IsSudoEnabled types.Bool   `tfsdk:"is_sudo_enabled" json:"isSudoEnabled,omitempty"`
	Label         types.String `tfsdk:"label" json:"label,omitempty"`
	ProjectId     types.String `tfsdk:"project_id" json:"projectId,omitempty"`
	State         types.String `tfsdk:"state" json:"state,omitempty"`
}

type sshGcpIamWriteJson struct {
	GroupKey      *string `json:"groupKey"`
	IsSudoEnabled *bool   `json:"isSudoEnabled,omitempty"`
	Label         *string `json:"label,omitempty"`
	State         string  `json:"state"`
}

type sshGcpIamWriteApi struct {
	Item *sshGcpIamWriteJson `json:"item"`
}

func NewSshGcpIamWrite() resource.Resource {
	return &sshGcpIamWrite{}
}

// Metadata implements resource.ResourceWithImportState.
func (*sshGcpIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_ssh_gcp"
}

// Schema implements resource.ResourceWithImportState.
func (*sshGcpIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Cloud SSH installation. 
		
Installing SSH allows you to manage access to your servers on Google Cloud.`,
		Attributes: map[string]schema.Attribute{
			"group_key": schema.StringAttribute{
				MarkdownDescription: `If present, Google Cloud instances will be grouped by the value of this tag. Access can be requested, in one request, to all instances with a shared tag value`,
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
				MarkdownDescription: "The Google Cloud project's alias (if available)",
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Google Cloud project ID",
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

func (r *sshGcpIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sshGcpIamWrite) getId(data any) *string {
	model, ok := data.(*sshGcpIamWriteModel)
	if !ok {
		return nil
	}

	str := fmt.Sprintf("%s%s", gcloudPrefix, model.ProjectId.ValueString())
	return &str
}

func (r *sshGcpIamWrite) getItemJson(json any) any {
	inner, ok := json.(*sshGcpIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *sshGcpIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := sshGcpIamWriteModel{}
	jsonv, ok := json.(*sshGcpIamWriteJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)

	projectId := strings.TrimPrefix(id, gcloudPrefix)
	data.ProjectId = types.StringValue(projectId)
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.GroupKey = types.StringNull()
	if jsonv.GroupKey != nil {
		group := types.StringValue(*jsonv.GroupKey)
		data.GroupKey = group
	}

	data.IsSudoEnabled = types.BoolNull()
	if jsonv.IsSudoEnabled != nil {
		isSudoEnabled := types.BoolValue(*jsonv.IsSudoEnabled)
		data.IsSudoEnabled = isSudoEnabled
	}

	return &data
}

func (r *sshGcpIamWrite) toJson(data any) any {
	json := sshGcpIamWriteJson{}

	datav, ok := data.(*sshGcpIamWriteModel)
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
func (s *sshGcpIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json sshGcpIamWriteApi
	var data sshGcpIamWriteModel
	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *sshGcpIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sshGcpIamWriteApi{}, &sshGcpIamWriteModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *sshGcpIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &sshGcpIamWriteModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *sshGcpIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &sshGcpIamWriteApi{}, &sshGcpIamWriteModel{})
}

func (s *sshGcpIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
