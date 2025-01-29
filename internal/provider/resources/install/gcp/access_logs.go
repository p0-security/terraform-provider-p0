package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpAccessLogs{}
var _ resource.ResourceWithImportState = &GcpAccessLogs{}
var _ resource.ResourceWithConfigure = &GcpAccessLogs{}

func NewGcpAccessLogs() resource.Resource {
	return &GcpAccessLogs{}
}

type GcpAccessLogs struct {
	installer *common.Install
}

func (r *GcpAccessLogs) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_access_logs"
}

func (r *GcpAccessLogs) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Google Cloud project, for access-log collection,
which enhances IAM assessment.

To use this resource, you must also:
- install the ` + "`p0_gcp_iam_assessment`" + ` resource, and
- grant P0 the ability to create logging sinks in your project.

Use the read-only attributes defined on ` + "`p0_gcp`" + ` to create the requisite Google Cloud infrastructure.

P0 recommends defining this infrastructure according to the example usage pattern.
`,
		Attributes: itemAttributes,
	}
}

func (r *GcpAccessLogs) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = newItemInstaller(AccessLogs, providerData)
}

func (s *GcpAccessLogs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpItemApi
	var data gcpItemModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *GcpAccessLogs) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpAccessLogs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpItemModel{})
}

func (s *GcpAccessLogs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpAccessLogs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
