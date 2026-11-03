package installmiroai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &MiroAiConnectorStaged{}
var _ resource.ResourceWithConfigure = &MiroAiConnectorStaged{}
var _ resource.ResourceWithImportState = &MiroAiConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &MiroAiConnectorStaged{}

type MiroAiConnectorStaged struct {
	installer *common.Install
	reader    *common.Install
}

func NewMiroAiConnectorStaged() resource.Resource {
	return &MiroAiConnectorStaged{}
}

func (*MiroAiConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_miro_ai_connector_staged"
}

func (*MiroAiConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Miro (AI) connector installation. Staging tells P0 where the connector will run. For Google Cloud, P0 answers with the service account the connector must run as.

Use ` + "`hosting`" + ` to deploy P0's Miro (AI) connector: a Lambda function on AWS, or a Cloud Run service that runs as ` + "`hosting.connector_service_account`" + ` on Google Cloud. Once the connector is deployed, create a ` + "`p0_miro_ai_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation, then add each Miro team it reaches with ` + "`p0_miro_ai_team`" + `.

Where a connector runs can't change once it is staged, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces the connector.

A connector that already exists in P0, such as one added in the P0 app, isn't staged again: the apply fails, and says to import it with ` + "`terraform import`" + `. Staging it again would return an installed connector to its staged state.

` + connectorRulesNote + `

**Prerequisite:** P0 invokes the connector as its own identity in your cloud. For AWS Lambda, ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. For Google Cloud Run, ` + "`p0_gcp`" + ` must be installed.

` + betaNote + `

` + installDocs,
		Attributes: connectorAttributes(
			`A name you choose for the connector, such as `+"`prod`"+`: `+itemIdRule,
			`Where the connector runs. The connector's Terraform deploys it there, and P0 invokes it there.`,
		),
	}
}

func (*MiroAiConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateConnector(ctx, req.Config, &resp.Diagnostics)
}

func (r *MiroAiConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = newConnectorInstaller(data)
	r.reader = newConnectorReader(data)
}

func (r *MiroAiConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json connectorApi
	var data connectorModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// Staging a connector that P0 holds would return an installed one to "stage", and
	// every Miro team behind it would stop working until it is verified
	// again. So this never stages over one, such as a connector installed in the P0
	// app. The rollback of p0_miro_ai_connector is another path: it stages a
	// connector that this resource created.
	id := data.Id.ValueString()
	refuseExistingItem(r.installer.ProviderData, installresources.Connector, "connector", id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, connectorToJson(&data))
	if resp.Diagnostics.HasError() {
		// A connector that P0 staged but returned in a shape this can't use, as without
		// its service account, is in P0 and not in state. It is removed, so that the
		// next apply finds nothing in its way and stages it again. A failure to remove
		// it changes nothing in the diagnostics: the next apply says to import it.
		_, _ = r.installer.ProviderData.Delete(itemPath(installresources.Connector, id))
	}
}

func (r *MiroAiConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.reader.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Every attribute that can be set requires replacement, so Terraform never updates a
// staged connector in place. Staging it again, as other staged resources' updates do,
// would return an installed connector to the "stage" state, so an update that reaches
// here fails rather than report a change it didn't make.
func (r *MiroAiConnectorStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Miro (AI) connectors can't be updated in place",
		"Every p0_miro_ai_connector_staged attribute requires replacement, so Terraform should never update one in place. Please report this as a provider bug.",
	)
}

func (r *MiroAiConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *MiroAiConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidConnectorId)
}
