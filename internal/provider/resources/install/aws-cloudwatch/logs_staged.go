package installawscloudwatch

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	logsource "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/log-source"
)

var _ resource.Resource = &LogsStaged{}
var _ resource.ResourceWithConfigure = &LogsStaged{}
var _ resource.ResourceWithImportState = &LogsStaged{}

type LogsStaged struct {
	installer *common.Install
}

func NewLogsStaged() resource.Resource {
	return &LogsStaged{}
}

func (*LogsStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_cloudwatch_logs_staged"
}

func (*LogsStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS CloudWatch Logs log source: one AWS account and region whose CloudTrail and RDS logs P0 reads from CloudWatch Logs, so that Access Logs can show what each request's access was used for.

Staging records the log source in P0, which answers with what the connector's Terraform needs: the connector's name, the role P0 invokes it with and that role's trust policy, and the connector's read policy. Deploy the connector with them, then create a ` + "`p0_aws_cloudwatch_logs`" + ` resource with the same ` + "`id`" + ` to complete the installation. See the ` + "`p0_aws_cloudwatch_logs`" + ` example for the whole chain.

` + logsource.BetaNote,
		Attributes: sourceAttributes(),
	}
}

func (r *LogsStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp))
}

func (r *LogsStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	logsource.EnsureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &sourceApi{}, &sourceModel{})
}

func (r *LogsStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sourceApi{}, &sourceModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged log source in place. Staging it again would also return an
// installed log source to the "stage" state.
func (r *LogsStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *LogsStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &sourceModel{})
}

func (r *LogsStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	logsource.ImportById(ctx, req, resp, path.Root("id"), idValidator())
}
