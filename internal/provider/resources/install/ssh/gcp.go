package installssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const gcloudPrefix = "gcloud:"

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &sshGcpIamWrite{}
var _ resource.ResourceWithConfigure = &sshGcpIamWrite{}
var _ resource.ResourceWithImportState = &sshGcpIamWrite{}

type sshGcpIamWrite struct {
	installer *installresources.Install
}

type sshGcpIamWriteModel struct {
	ProjectId types.String `tfsdk:"project_id" json:"projectId,omitempty"`
	State     types.String `tfsdk:"state" json:"state,omitempty"`
	Label     types.String `tfsdk:"label" json:"label,omitempty"`
}

type sshGcpIamWriteJson struct {
	State string  `json:"state"`
	Label *string `json:"label,omitempty"`
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
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Google Cloud project ID",
				Required:            true,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: installresources.StateMarkdownDescription,
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Google Cloud project's alias (if available)",
			},
		},
	}
}

func (r *sshGcpIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
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

	projectId := strings.TrimPrefix(id, gcloudPrefix)
	data.ProjectId = types.StringValue(projectId)
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.State = types.StringValue(jsonv.State)

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
