package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpIamWrite{}
var _ resource.ResourceWithImportState = &GcpIamWrite{}
var _ resource.ResourceWithConfigure = &GcpIamWrite{}

func NewGcpIamWrite() resource.Resource {
	return &GcpIamWrite{}
}

type GcpIamWrite struct {
	installer *installresources.Install
}

func (r *GcpIamWrite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_iam_write"
}

func (r *GcpIamWrite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Google Cloud project, for IAM management.

To use this resource, you must also:
- install the ` + "`p0_gcp_iam_write_staged`" + ` resource
- create a custom role allowing IAM-management operations,
- grant this custom role to P0's service account,
- grant the ` + "`iam.securityAdmin`" + ` role to P0's service account.

Use the read-only attributes defined on ` + "`p0_gcp_iam_write_staged`" + ` to create the requisite Google Cloud infrastructure.

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: itemAttributes,
	}
}

func (r *GcpIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = newItemInstaller(installresources.IamWrite, providerData)
}

func (s *GcpIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gcpItemModel{})
}

func (s *GcpIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
