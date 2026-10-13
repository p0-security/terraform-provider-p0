package installharbor

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &HarborConnectorStaged{}
var _ resource.ResourceWithConfigure = &HarborConnectorStaged{}
var _ resource.ResourceWithImportState = &HarborConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &HarborConnectorStaged{}

type HarborConnectorStaged struct {
	installer *common.Install
}

func NewHarborConnectorStaged() resource.Resource {
	return &HarborConnectorStaged{}
}

func (*HarborConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_harbor_connector_staged"
}

func (*HarborConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Harbor connector installation. Staging tells P0 where the connector will run. For Google Cloud, P0 answers with the service account the connector must run as.

Use ` + "`hosting`" + ` to deploy P0's Harbor connector: a Lambda function on AWS, or a Cloud Run service that runs as ` + "`hosting.connector_service_account`" + ` on Google Cloud. Once the connector is deployed, create a ` + "`p0_harbor_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation.

Where a connector runs can't change once it is staged, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces the connector.

` + connectorRulesNote + `

**Prerequisite:** P0 invokes the connector as its own identity in your cloud. For AWS Lambda, ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. For Google Cloud Run, ` + "`p0_gcp`" + ` must be installed.

` + previewNote,
		Attributes: connectorAttributes(
			`A name you choose for the connector, such as `+"`prod`"+`: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters`,
			`Where the connector runs. The connector's Terraform deploys it there, and P0 invokes it there.`,
		),
	}
}

func (*HarborConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateConnector(ctx, req.Config, &resp.Diagnostics)
}

func (r *HarborConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newConnectorInstaller(internal.Configure(&req, resp))
}

func (r *HarborConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json connectorApi
	var data connectorModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, connectorToJson(&data))
}

func (r *HarborConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Every attribute that can be set requires replacement, so Terraform never updates a
// staged connector in place. Staging it again, as other staged resources' updates do,
// would also return an installed connector to the "stage" state.
func (r *HarborConnectorStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *HarborConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *HarborConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidConnectorId)
}
