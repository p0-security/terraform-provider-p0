package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpIamWriteStaged{}
var _ resource.ResourceWithImportState = &GcpIamWriteStaged{}
var _ resource.ResourceWithConfigure = &GcpIamWriteStaged{}

func NewGcpIamWriteStaged() resource.Resource {
	return &GcpIamWriteStaged{}
}

type GcpIamWriteStaged struct {
	installer *common.Install
}

type gcpIamWriteStagedModel struct {
	Project        string       `tfsdk:"project"`
	State          types.String `tfsdk:"state"`
	PredefinedRole types.String `tfsdk:"predefined_role"`
	Permissions    types.List   `tfsdk:"permissions"`
	CustomRole     types.Object `tfsdk:"custom_role"`
}

type gcpIamWriteStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
	Metadata gcpPermissionsMetadataWithPredefinedRole `json:"metadata"`
}

func (r *GcpIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_iam_write_staged"
}

func (r *GcpIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Google Cloud project, for IAM management.

For instructions on using this resource, see the documentation for ` + "`p0_gcp_iam_write`.",
		Attributes: map[string]schema.Attribute{
			// In P0 we would name this 'id' or 'project_id'; it is named 'project' here to align with Terraform's naming for
			// Google Cloud resources
			"project":         projectAttribute,
			"state":           common.StateAttribute,
			"permissions":     permissions("IAM management"),
			"predefined_role": predefinedRole,
			"custom_role":     customRole,
		},
	}
}

func (r *GcpIamWriteStaged) getId(data any) *string {
	model, ok := data.(*gcpIamWriteStagedModel)
	if !ok {
		return nil
	}
	return &model.Project
}

func (r *GcpIamWriteStaged) getItemJson(json any) any {
	return json
}

func (r *GcpIamWriteStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpIamWriteStagedModel{}
	jsonv, ok := json.(*gcpIamWriteStagedApi)
	if !ok {
		return nil
	}

	data.Project = id
	data.State = types.StringValue(jsonv.Item.State)
	data.PredefinedRole = types.StringValue(jsonv.Metadata.PredefinedRole)

	permissions, pDiags := types.ListValueFrom(ctx, types.StringType, jsonv.Metadata.Permissions)
	if pDiags.HasError() {
		diags.Append(pDiags...)
		return nil
	}
	data.Permissions = permissions

	customRole, crDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"id":   types.StringType,
		"name": types.StringType,
	}, jsonv.Metadata.CustomRole)
	if crDiags.HasError() {
		diags.Append(crDiags...)
		return nil
	}
	data.CustomRole = customRole

	return &data
}

func (r *GcpIamWriteStaged) toJson(data any) any {
	json := gcpIamWriteStagedApi{}
	return &json.Item
}

func (r *GcpIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *GcpIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpIamWriteStagedApi
	var data gcpIamWriteStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
}

func (s *GcpIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpIamWriteStagedApi{}, &gcpIamWriteStagedModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *GcpIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpIamWriteStagedModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *GcpIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpIamWriteStagedApi{}, &gcpIamWriteStagedModel{})
}

func (s *GcpIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
