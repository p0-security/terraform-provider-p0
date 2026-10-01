package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &ConnectorStaged{}
var _ resource.ResourceWithConfigure = &ConnectorStaged{}
var _ resource.ResourceWithImportState = &ConnectorStaged{}

type ConnectorStaged struct {
	installer *common.Install
}

func NewConnectorStaged() resource.Resource {
	return &ConnectorStaged{}
}

func (*ConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_connector_staged"
}

func (*ConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Databricks connector. The connector is the ` + "`p0-connector-databricks`" + ` Lambda that P0 invokes to manage Databricks, one per AWS account. It runs outside any VPC and holds no secret: it reaches each Databricks account by exchanging its AWS identity for a Databricks token.

Staging records the connector in P0 and checks its domain pattern before you deploy the Lambda with it. Then deploy the Lambda and its role, enable outbound web identity federation for the AWS account, and create a ` + "`p0_databricks_connector`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0's AWS integration must be installed for the connector's AWS account (for example via the ` + "`p0_aws_iam_write`" + ` resource). P0 invokes the connector as that installation's role.

` + notePreview,
		Attributes: connectorAttributes(),
	}
}

func (r *ConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newConnectorInstaller(internal.Configure(&req, resp))
}

func (r *ConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *ConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *ConnectorStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	stage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *ConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *ConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
