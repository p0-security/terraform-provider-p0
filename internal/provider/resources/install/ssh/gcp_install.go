package installssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &gcpSshIamWrite{}
var _ resource.ResourceWithConfigure = &gcpSshIamWrite{}
var _ resource.ResourceWithImportState = &gcpSshIamWrite{}

type gcpSshIamWrite struct {
	installer *installresources.Install
}

type gcpSshIamWriteModel struct {
	ProjectId types.String `tfsdk:"project_id" json:"projectId,omitempty"`
	State     types.String `tfsdk:"state" json:"state,omitempty"`
	Label     types.String `tfsdk:"label" json:"label,omitempty"`
}

type gcpSshIamWriteJson struct {
	State string  `json:"state"`
	Label *string `json:"label,omitempty"`
}

type gcpSshIamWriteApi struct {
	Item *gcpSshIamWriteJson `json:"item"`
}

func NewGcpSshIamWrite() resource.Resource {
	return &gcpSshIamWrite{}
}

// Metadata implements resource.ResourceWithImportState.
func (*gcpSshIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_ssh_install"
}

// Schema implements resource.ResourceWithImportState.
func (*gcpSshIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Note that the TF doc generator clobbers _most_ underscores :(
		MarkdownDescription: `A Google Cloud SSH installation. 
		
Installing SSH allows you to manage access to your servers on Google Cloud.`,
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Google Cloud project ID.",
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

func (r *gcpSshIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		ProviderData: data,
		GetItemPath:  r.getItemPath,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
	if data == nil {
		return
	}
}

func (r *gcpSshIamWrite) getId(data any) *string {
	model, ok := data.(*gcpSshIamWriteModel)
	if !ok {
		return nil
	}

	str := model.ProjectId.ValueString()
	return &str
}

func (r *gcpSshIamWrite) getItemPath(id string) string {
	return fmt.Sprintf("integrations/%s/config/%s/gcloud:%s", SshKey, installresources.IamWrite, id)
}

func (r *gcpSshIamWrite) getItemJson(json any) any {
	inner, ok := json.(*gcpSshIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *gcpSshIamWrite) fromJson(id string, json any) any {
	data := gcpSshIamWriteModel{}
	jsonv, ok := json.(*gcpSshIamWriteJson)
	if !ok {
		return nil
	}

	data.ProjectId = types.StringValue(id)
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *gcpSshIamWrite) toJson(data any) any {
	json := gcpSshIamWriteJson{}

	datav, ok := data.(*gcpSshIamWriteModel)
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
func (s *gcpSshIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gcpSshIamWriteModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	throwaway_response := struct{}{}
	err := s.installer.ProviderData.Post("integrations/ssh/config", struct{}{}, &throwaway_response)
	if err != nil {
		if !strings.Contains(err.Error(), "409 Conflict") {
			resp.Diagnostics.AddError("Failed to install IAM write", err.Error())
			return
		}
	}

	s.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSshIamWriteApi{}, &gcpSshIamWriteModel{})
}

func (s *gcpSshIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpSshIamWriteApi{}, &gcpSshIamWriteModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *gcpSshIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gcpSshIamWriteModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := s.getId(&state)
	path := s.getItemPath(*id)
	s.installer.ProviderData.Delete(path)
}

// Update implements resource.ResourceWithImportState.
func (s *gcpSshIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSshIamWriteApi{}, &gcpSshIamWriteModel{})
}

func (s *gcpSshIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
