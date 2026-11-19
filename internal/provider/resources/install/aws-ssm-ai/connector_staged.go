package installawsssmai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &ConnectorStaged{}
var _ resource.ResourceWithConfigure = &ConnectorStaged{}
var _ resource.ResourceWithImportState = &ConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &ConnectorStaged{}

type ConnectorStaged struct {
	installer *common.Install
}

func NewConnectorStaged() resource.Resource {
	return &ConnectorStaged{}
}

func (*ConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_ssm_ai_connector_staged"
}

func (*ConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS SSM (AI) connector. The connector is the ` + "`" + connectorFunctionName + "`" + ` Lambda that P0 invokes to give AI agents SSH access to EC2 instances, one per AWS account. It reaches every instance that SSM manages in its account through SSM, and keeps each agent's SSH key in AWS Secrets Manager, in its own account.

Its role runs commands through the ` + "`AWS-RunShellScript`" + ` document, which runs them as root, so the connector can run any command as root on every SSM-managed instance in the account. Protect the Lambda and its role as you would root on those instances.

Staging records the connector in P0 before you deploy the Lambda. Then deploy the Lambda and its role, let P0's AWS IAM management role invoke the Lambda, and create a ` + "`p0_aws_ssm_ai_connector`" + ` resource with the same ` + "`id`" + `, ` + "`region`" + ` and ` + "`vault`" + ` to complete the installation.

**Prerequisites:** P0 needs these integrations for the connector's AWS account. Staging fails until the first two are installed.

- AWS IAM management (see ` + "`p0_aws_iam_write`" + `). P0 invokes the connector as its role.
- AWS OIDC (see ` + "`p0_aws_oidc_identity`" + `). An agent's MCP server reads the agent's key through it.
- AWS Secrets Manager (see ` + "`p0_aws_secrets_manager`" + `), which ` + "`vault`" + ` names. The connector keeps the keys there.

Destroying the connector also needs AWS IAM management and AWS OIDC, so keep them until it is gone. When one configuration holds them, add them to this resource's ` + "`depends_on`" + `, so that Terraform destroys the connector first.

` + previewNote + `

` + docsNote,
		Attributes: connectorStagedAttributes(),
	}
}

func (r *ConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp), connectorStagedFromJson, connectorStagedToJson)
}

func connectorStagedFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	model := newConnectorStagedModel(id, item)
	return &model
}

func connectorStagedToJson(data any) any {
	model, ok := data.(*connectorStagedModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

func (*ConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateConnectorConfig(ctx, req.Config, &resp.Diagnostics)
}

func (r *ConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	ensureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorStagedModel{})
}

func (r *ConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorStagedModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged connector in place. Staging it again would also return an
// installed connector to the "stage" state.
func (r *ConnectorStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *ConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorStagedModel{})
}

func (r *ConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, awsAccountIdValidator())
}
