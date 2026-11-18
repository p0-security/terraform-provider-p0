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

var _ resource.Resource = &Logs{}
var _ resource.ResourceWithConfigure = &Logs{}
var _ resource.ResourceWithImportState = &Logs{}
var _ resource.ResourceWithModifyPlan = &Logs{}

type Logs struct {
	installer *common.Install
}

func NewLogs() resource.Resource {
	return &Logs{}
}

func (*Logs) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_cloudwatch_logs"
}

func (*Logs) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS CloudWatch Logs log source installation: one AWS account and region whose CloudTrail and RDS logs P0 reads, so that Access Logs can show what each request's access was used for.

**Important:** Before creating this resource you must stage the log source with ` + "`p0_aws_cloudwatch_logs_staged`" + `, and deploy the following in the staged account and region, with the names and policies that staging returns:

- The connector's Lambda function, named ` + "`connector_name`" + `, from P0's pinned connector image copied into ECR. It runs outside any VPC.
- The connector's execution role, with ` + "`execution_policy`" + `.
- P0's invoke role, named ` + "`invoke_role_name`" + `, with ` + "`invoke_trust_policy`" + ` and a policy that allows ` + "`lambda:InvokeFunction`" + ` on the connector and nothing else.

If ` + "`cloudtrail`" + ` is set, its trail must already deliver to the log group: creating this resource has P0 check, through the connector, that the log group holds a CloudTrail event from the last 24 hours and that the trail's coverage matches. P0 retries for about 15 seconds while new IAM roles and the function take effect, then the apply fails with P0's message. If the first apply is denied access, apply again.

A log source that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + logsource.BetaNote,
		Attributes: sourceAttributes(),
	}
}

func (r *Logs) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp))
}

func (r *Logs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	logsource.StageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &sourceApi{}, &sourceModel{})
}

func (r *Logs) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sourceApi{}, &sourceModel{})
}

func (*Logs) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	logsource.PlanFinishingInstall(ctx, req, resp)
}

// Every attribute requires replacement, so an update only finishes an install
// that P0 hasn't.
func (r *Logs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	logsource.UpdateFromInstallState(ctx, r.installer, req, resp, &sourceApi{}, &sourceModel{})
}

// Returns the log source to the "stage" state, so that the staged resource
// deletes it.
func (r *Logs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &sourceModel{})
}

func (r *Logs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	logsource.ImportById(ctx, req, resp, path.Root("id"), idValidator())
}
