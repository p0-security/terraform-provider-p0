package installgcpcloudlogging

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	logsource "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/log-source"
)

var _ resource.Resource = &OrganizationStaged{}
var _ resource.ResourceWithConfigure = &OrganizationStaged{}
var _ resource.ResourceWithImportState = &OrganizationStaged{}

type OrganizationStaged struct {
	installer *common.Install
}

func NewOrganizationStaged() resource.Resource {
	return &OrganizationStaged{}
}

func (*OrganizationStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_organization_cloud_logging_staged"
}

func (*OrganizationStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Google Cloud Logging log source for your whole Google Cloud organization: P0 reads the Cloud Audit Logs of every project in the organization that P0's Google Cloud integration is installed for, so that Access Logs can show what each request's access was used for. A project's own log source, ` + "`p0_gcp_cloud_logging`" + `, takes precedence over it for that project.

P0 has one organization log source. Staging records it in P0, which answers with what the connector's Terraform needs: the connector's Cloud Run service name, its service account and its region. Deploy the connector with them, then create a ` + "`p0_gcp_organization_cloud_logging`" + ` resource with the same ` + "`host_project_id`" + ` to complete the installation. See the ` + "`p0_gcp_organization_cloud_logging`" + ` example for the whole chain.

Import it with its host project ID.

` + prerequisiteNote + `

` + logsource.BetaNote,
		Attributes: connectorAttributes(),
	}
}

func (r *OrganizationStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp), organizationComponent, singletonGetId, organizationStagedFromJson, organizationStagedToJson)
}

func (r *OrganizationStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	logsource.EnsureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &sourceApi{}, &organizationStagedModel{})
}

func (r *OrganizationStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sourceApi{}, &organizationStagedModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged log source in place. Staging it again would also return an
// installed log source to the "stage" state.
func (r *OrganizationStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *OrganizationStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &organizationStagedModel{})
}

func (r *OrganizationStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	logsource.ImportById(ctx, req, resp, path.Root("host_project_id"), projectIdValidator())
}
