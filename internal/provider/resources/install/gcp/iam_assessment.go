package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
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
	installer *common.Install
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
				MarkdownDescription: common.StateMarkdownDescription,
			},
		},
	}
}

func (r *GcpIamAssessment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = newItemInstaller(installresources.IamAssessment, providerData)
}

func (s *GcpIamAssessment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamAssessment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamAssessment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gcpItemModel{})
}

func (s *GcpIamAssessment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamAssessment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
