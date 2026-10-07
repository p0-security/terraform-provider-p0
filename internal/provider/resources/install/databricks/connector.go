package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &Connector{}
var _ resource.ResourceWithConfigure = &Connector{}
var _ resource.ResourceWithImportState = &Connector{}
var _ resource.ResourceWithModifyPlan = &Connector{}

type Connector struct {
	installer *common.Install
}

func NewConnector() resource.Resource {
	return &Connector{}
}

func (*Connector) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_connector"
}

func (*Connector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Databricks connector installation.

The connector lets P0 grant Unity Catalog privileges, workspace admin and account admin in your Databricks accounts. After you install it, add each Databricks account with ` + "`p0_databricks_account`" + `, then the account's workspaces with ` + "`p0_databricks_workspace`" + ` and their catalogs with ` + "`p0_databricks_catalog`" + `.

**Important:** Before creating this resource you must stage the connector with ` + "`p0_databricks_connector_staged`" + `, and set up the following in the staged AWS account and region:

- Outbound identity federation, enabled for the AWS account (` + "`aws_iam_outbound_web_identity_federation`" + `).
- An IAM role named ` + "`p0-connector-databricks`" + ` that Lambda can assume. Each Databricks account's federation policy trusts this role's ARN.
- A policy on that role that allows ` + "`sts:GetWebIdentityToken`" + ` only for the audience ` + "`" + FederationAudience + "`" + ` (this resource's ` + "`federation_audience`" + `).
- The ` + "`p0-connector-databricks`" + ` Lambda, running as that role, with its ` + "`DOMAIN_PATTERN`" + ` environment variable set to ` + "`domain_pattern`" + `.
- A policy on P0's AWS integration role that allows ` + "`lambda:InvokeFunction`" + ` on the Lambda.

A connector that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + common.NotePreview,
		Attributes: connectorAttributes(),
	}
}

func (r *Connector) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newConnectorInstaller(internal.Configure(&req, resp))
}

func (r *Connector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *Connector) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

func (*Connector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	common.PlanFinishingInstall(ctx, req, resp)
}

// Every attribute requires replacement, so an update only finishes an install
// that P0 hasn't.
func (r *Connector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	upsertFromState(ctx, r.installer, req, resp, &connectorApi{}, &connectorModel{})
}

// Returns the connector to the "stage" state, so that the staged resource
// deletes it.
func (r *Connector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *Connector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
