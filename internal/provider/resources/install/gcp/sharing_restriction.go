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
var _ resource.Resource = &GcpSharingRestriction{}
var _ resource.ResourceWithImportState = &GcpSharingRestriction{}
var _ resource.ResourceWithConfigure = &GcpSharingRestriction{}

func NewGcpSharingRestriction() resource.Resource {
	return &GcpSharingRestriction{}
}

type GcpSharingRestriction struct {
	installer *installresources.Install
}

func (r *GcpSharingRestriction) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_sharing_restriction"
}

func (r *GcpSharingRestriction) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Validates installation of a domain-restricted sharing policy, preventing privilege
escalation via the P0 IAM-management integration.

To use this resource, you must:
- install the ` + "`p0_gcp_iam_write`" + ` resource, and
- create a domain-restricted-sharing organization policy

P0 recommends defining this infrastructure according to the pattern in the example usage.
`,
		Attributes: itemAttributes,
	}
}

func (r *GcpSharingRestriction) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = newItemInstaller(SharingRestriction, providerData)
}

func (s *GcpSharingRestriction) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpItemApi
	var data gcpItemModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *GcpSharingRestriction) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpSharingRestriction) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpItemModel{})
}

func (s *GcpSharingRestriction) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpItemApi{}, &gcpItemModel{})
}

func (s *GcpSharingRestriction) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
