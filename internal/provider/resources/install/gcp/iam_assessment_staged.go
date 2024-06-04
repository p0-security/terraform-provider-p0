package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpIamAssessmentStaged{}
var _ resource.ResourceWithImportState = &GcpIamAssessmentStaged{}
var _ resource.ResourceWithConfigure = &GcpIamAssessmentStaged{}

func NewGcpIamAssessmentStaged() resource.Resource {
	return &GcpIamAssessmentStaged{}
}

type GcpIamAssessmentStaged struct {
	installer *installresources.Install
}

type gcpIamAssessmentStagedModel struct {
	Project     string       `tfsdk:"project"`
	State       types.String `tfsdk:"state"`
	Permissions types.List   `tfsdk:"permissions"`
	CustomRole  types.Object `tfsdk:"custom_role"`
}

type gcpIamAssessmentStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
	Metadata gcpPermissionsMetadata `json:"metadata"`
}

func (r *GcpIamAssessmentStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_iam_assessment_staged"
}

func (r *GcpIamAssessmentStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged installation of P0, on a single Google Cloud project, for IAM assessment.

For instructions on using this resource, see the documentation for ` + "`p0_gcp_iam_assessment`.",
		Attributes: map[string]schema.Attribute{
			// In P0 we would name this 'id' or 'project_id'; it is named 'project' here to align with Terraform's naming for
			// Google Cloud resources
			"project": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the Google Cloud project to assess with P0",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: installresources.StateMarkdownDescription,
			},
			"permissions": permissions("IAM assessment"),
			"custom_role": customRole,
		},
	}
}

func (r *GcpIamAssessmentStaged) getId(data any) *string {
	model, ok := data.(*gcpIamAssessmentStagedModel)
	if !ok {
		return nil
	}
	return &model.Project
}

func (r *GcpIamAssessmentStaged) getItemJson(json any) any {
	return json
}

func (r *GcpIamAssessmentStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpIamAssessmentStagedModel{}
	jsonv, ok := json.(*gcpIamAssessmentStagedApi)
	if !ok {
		return nil
	}

	data.Project = id
	data.State = types.StringValue(jsonv.Item.State)

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

func (r *GcpIamAssessmentStaged) toJson(data any) any {
	json := gcpIamAssessmentStagedApi{}
	return &json.Item
}

func (r *GcpIamAssessmentStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		Integration:  GcpKey,
		Component:    installresources.IamAssessment,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *GcpIamAssessmentStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpIamAssessmentStagedApi
	var data gcpIamAssessmentStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *GcpIamAssessmentStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpIamAssessmentStagedApi{}, &gcpIamAssessmentStagedModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *GcpIamAssessmentStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpIamAssessmentStagedModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *GcpIamAssessmentStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpIamAssessmentStagedApi{}, &gcpIamAssessmentStagedModel{})
}

func (s *GcpIamAssessmentStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
