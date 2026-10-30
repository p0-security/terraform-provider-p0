package installredshiftai

import (
	"context"

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
	resp.TypeName = req.ProviderTypeName + "_redshift_ai_connector_staged"
}

func (*ConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Redshift (AI) connector. The connector is the ` + "`p0-connector-redshift-ai-aws`" + ` Lambda that P0 invokes to give AI agents access to Amazon Redshift, one per AWS account. It runs outside any VPC and holds no secret: it signs in to each cluster and Serverless workgroup in its account with its IAM role, and runs statements through the Redshift Data API.

Staging records the connector in P0 before you deploy the Lambda. Then deploy the Lambda and its role, let P0's AWS integration role invoke the Lambda, and create a ` + "`p0_redshift_ai_connector`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0's AWS integration must be installed for the connector's AWS account (for example via the ` + "`p0_aws_iam_write`" + ` resource). P0 invokes the connector as that installation's role.

` + betaNote + `

` + docsNote,
		Attributes: connectorAttributes(),
	}
}

func (r *ConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newConnectorInstaller(internal.Configure(&req, resp))
}

func (r *ConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	ensureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *ConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged connector in place. Staging it again would also return an
// installed connector to the "stage" state.
func (r *ConnectorStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *ConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *ConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, awsAccountIdValidator())
}
