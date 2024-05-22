package installgcp

import (
	"context"

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
var _ resource.Resource = &GcpIamAssessment{}
var _ resource.ResourceWithImportState = &GcpIamAssessment{}
var _ resource.ResourceWithConfigure = &GcpIamAssessment{}

func NewGcpIamAssessment() resource.Resource {
	return &GcpIamAssessment{}
}

type GcpIamAssessment struct {
	installer *installresources.Install
}

type gcpIamAssessmentModel struct {
	Project string       `tfsdk:"project"`
	State   types.String `tfsdk:"state"`
}

type gcpIamAssessmentJson struct {
	State string `json:"state"`
}

type gcpIamAssessmentApi struct {
	Item gcpIamAssessmentJson `json:"item"`
}

func (r *GcpIamAssessment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_iam_assessment"
}

func (r *GcpIamAssessment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Google Cloud project, for IAM assessment.

To use this resource, you must also:
- install the ` + "`p0_gcp_iam_assessment_staged`" + ` resource,
- create a custom role allowing IAM-assessment operations, and
- grant this custom role to P0's service account.

Use the read-only attributes defined on ` + "`p0_gcp_iam_assessment_staged`" + ` to create the requisite Google Cloud infrastructure.

P0 recommends defining this infrastructure according to the pattern in the example usage.
`,
		Attributes: map[string]schema.Attribute{
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
		},
	}
}

func (r *GcpIamAssessment) getId(data any) *string {
	model, ok := data.(*gcpIamAssessmentModel)
	if !ok {
		return nil
	}
	return &model.Project
}

func (r *GcpIamAssessment) getItemJson(json any) any {
	inner, ok := json.(*gcpIamAssessmentApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *GcpIamAssessment) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpIamAssessmentModel{}
	jsonv, ok := json.(*gcpIamAssessmentJson)
	if !ok {
		return nil
	}

	data.Project = id
	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *GcpIamAssessment) toJson(data any) any {
	json := gcpIamAssessmentJson{}

	// can omit state here as it's filled by the backend
	return json
}

func (r *GcpIamAssessment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (s *GcpIamAssessment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpIamAssessmentApi{}, &gcpIamAssessmentModel{})
}

func (s *GcpIamAssessment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpIamAssessmentApi{}, &gcpIamAssessmentModel{})
}

func (s *GcpIamAssessment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gcpIamAssessmentModel{})
}

func (s *GcpIamAssessment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpIamAssessmentApi{}, &gcpIamAssessmentModel{})
}

func (s *GcpIamAssessment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
