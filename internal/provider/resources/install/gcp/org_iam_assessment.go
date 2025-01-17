package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpOrgIamAssessment{}
var _ resource.ResourceWithImportState = &GcpOrgIamAssessment{}
var _ resource.ResourceWithConfigure = &GcpOrgIamAssessment{}

func NewGcpOrgIamAssessment() resource.Resource {
	return &GcpOrgIamAssessment{}
}

type GcpOrgIamAssessment struct {
	installer *common.Install
}

type gcpOrgIamAssessmentModel struct {
	State types.String `tfsdk:"state"`
}

func (r *GcpOrgIamAssessment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_organization_iam_assessment"
}

func (r *GcpOrgIamAssessment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on an entire Google Cloud organization, for IAM assessment.

To use this resource, you must also:
- create a custom role allowing IAM-assessment operations, and
- grant this custom role to P0's service account.

Use the read-only attributes defined on ` + "`p0_gcp`" + ` to create the requisite Google Cloud infrastructure.

P0 recommends defining this infrastructure according to the example usage pattern.`,
		Attributes: map[string]schema.Attribute{
			"state": stateAttribute,
		},
	}
}

func (r *GcpOrgIamAssessment) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpOrgIamAssessmentModel{}
	jsonv, ok := json.(*gcpItemJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *GcpOrgIamAssessment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpKey,
		Component:    OrgIamAssessment,
		ProviderData: providerData,
		GetId:        singletonGetId,
		GetItemJson:  itemGetItemJson,
		FromJson:     r.fromJson,
		ToJson:       itemToJson,
	}
}

func (s *GcpOrgIamAssessment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model gcpOrgIamAssessmentModel
	var json gcpItemApi
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &model)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &model)
}

func (s *GcpOrgIamAssessment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpItemApi{}, &gcpOrgIamAssessmentModel{})
}

func (s *GcpOrgIamAssessment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpOrgIamAssessmentModel{})
}

func (s *GcpOrgIamAssessment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpOrgIamAssessmentModel{})
}

func (s *GcpOrgIamAssessment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Empty(), req, resp)
}
