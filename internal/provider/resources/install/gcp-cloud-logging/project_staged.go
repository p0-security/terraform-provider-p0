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

var _ resource.Resource = &ProjectStaged{}
var _ resource.ResourceWithConfigure = &ProjectStaged{}
var _ resource.ResourceWithImportState = &ProjectStaged{}

type ProjectStaged struct {
	installer *common.Install
}

func NewProjectStaged() resource.Resource {
	return &ProjectStaged{}
}

func (*ProjectStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_cloud_logging_staged"
}

func (*ProjectStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Google Cloud Logging log source for one project: P0 reads the project's Cloud Audit Logs, so that Access Logs can show what each request's access was used for.

Staging records the log source in P0, which answers with what the connector's Terraform needs: the connector's Cloud Run service name, its service account and its region. Deploy the connector with them, then create a ` + "`p0_gcp_cloud_logging`" + ` resource with the same ` + "`project_id`" + ` to complete the installation. See the ` + "`p0_gcp_cloud_logging`" + ` example for the whole chain.

` + prerequisiteNote + `

` + logsource.BetaNote,
		Attributes: projectAttributes(),
	}
}

func (r *ProjectStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp), projectComponent, projectGetId, projectStagedFromJson, projectStagedToJson)
}

func (r *ProjectStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	logsource.EnsureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &sourceApi{}, &projectStagedModel{})
}

func (r *ProjectStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sourceApi{}, &projectStagedModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged log source in place. Staging it again would also return an
// installed log source to the "stage" state.
func (r *ProjectStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *ProjectStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &projectStagedModel{})
}

func (r *ProjectStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	logsource.ImportById(ctx, req, resp, path.Root("project_id"), projectIdValidator())
}
