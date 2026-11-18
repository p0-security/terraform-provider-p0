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

var _ resource.Resource = &Project{}
var _ resource.ResourceWithConfigure = &Project{}
var _ resource.ResourceWithImportState = &Project{}
var _ resource.ResourceWithModifyPlan = &Project{}

type Project struct {
	installer *common.Install
}

func NewProject() resource.Resource {
	return &Project{}
}

func (*Project) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_cloud_logging"
}

func (*Project) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := projectAttributes()
	attributes["connector_service_uri"] = logsource.GeneratedAttribute(connectorServiceUriDescription)
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Cloud Logging log source installation for one project: P0 reads the project's Cloud Audit Logs, so that Access Logs can show what each request's access was used for.

**Important:** Before creating this resource you must stage the log source with ` + "`p0_gcp_cloud_logging_staged`" + `, and deploy the following in the host project, with the names that staging returns:

- The connector's service account, ` + "`connector_service_account`" + `, granted ` + "`roles/logging.privateLogViewer`" + ` on the project, so that it can read Data Access logs.
- The connector's Cloud Run service, ` + "`connector_service_name`" + ` in ` + "`region`" + `, from P0's pinned connector image, running as that service account. It needs no VPC access.
- ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the service for P0's service account, the ` + "`service_account_email`" + ` of ` + "`p0_gcp`" + `.

Creating this resource has P0 find the service's URL and check, through the connector, that it can read the project's Cloud Audit Logs. P0 retries for about 15 seconds while new IAM bindings take effect, then the apply fails with P0's message. If the first apply is denied access, apply again.

A log source that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + prerequisiteNote + `

` + logsource.BetaNote,
		Attributes: attributes,
	}
}

func (r *Project) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp), projectComponent, projectGetId, projectFromJson, projectToJson)
}

func (r *Project) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	logsource.StageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &sourceApi{}, &projectModel{})
}

func (r *Project) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sourceApi{}, &projectModel{})
}

// P0 finds the connector's URL when it installs the log source, so a log
// source it hasn't installed has none yet.
func (*Project) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	logsource.PlanFinishingInstall(ctx, req, resp, path.Root("connector_service_uri"))
}

// Every attribute requires replacement, so an update only finishes an install
// that P0 hasn't.
func (r *Project) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	logsource.UpdateFromInstallState(ctx, r.installer, req, resp, &sourceApi{}, &projectModel{})
}

// Returns the log source to the "stage" state, so that the staged resource
// deletes it.
func (r *Project) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &projectModel{})
}

func (r *Project) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	logsource.ImportById(ctx, req, resp, path.Root("project_id"), projectIdValidator())
}
