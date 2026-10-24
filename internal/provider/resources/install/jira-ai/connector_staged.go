package installjiraai

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &JiraAiConnectorStaged{}
var _ resource.ResourceWithConfigure = &JiraAiConnectorStaged{}
var _ resource.ResourceWithImportState = &JiraAiConnectorStaged{}

// A Google Cloud-only copy of Harbor's p0_harbor_connector_staged
// (harbor/connector_staged.go).
type JiraAiConnectorStaged struct {
	installer *common.Install
	reader    *common.Install
}

func NewJiraAiConnectorStaged() resource.Resource {
	return &JiraAiConnectorStaged{}
}

func (*JiraAiConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_jira_ai_connector_staged"
}

func (*JiraAiConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Jira connector installation, for Jira agent access. Staging tells P0 where the connector will run, and P0 answers with the service account the connector must run as.

Use ` + "`hosting`" + ` to deploy P0's Jira connector: a Cloud Run service that runs as ` + "`hosting.connector_service_account`" + `. Once the connector is deployed, create a ` + "`p0_jira_ai_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation, then add each Jira site it reaches with ` + "`p0_jira_ai_site`" + `.

Where a connector runs can't change once it is staged, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces the connector.

A connector that already exists in P0, such as one added in the P0 app, isn't staged again: the apply fails, and says to import it with ` + "`terraform import`" + `. Staging it again would return an installed connector to its staged state.

**Prerequisite:** P0 invokes the connector as its own Google Cloud service account, so ` + "`p0_gcp`" + ` must be installed.

` + betaNote,
		Attributes: connectorAttributes(
			`A name you choose for the connector, such as `+"`prod`"+`. Each site names its connector by it. Use `+itemIdRule+`.`,
			`Where the connector runs. The connector's Terraform deploys it there, and P0 invokes it there.`,
		),
	}
}

func (r *JiraAiConnectorStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = newConnectorInstaller(data)
	r.reader = newConnectorReader(data)
}

// A copy of Harbor's p0_harbor_connector_staged Create (harbor/connector_staged.go).
func (r *JiraAiConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json connectorApi
	var data connectorModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// Staging over a connector P0 holds would return it to "stage", and every site
	// behind it would stop working until it's verified again. p0_jira_ai_connector's
	// Delete stages its connector again on purpose, by its own path.
	id := data.Id.ValueString()
	refuseExistingItem(r.installer.ProviderData, installresources.Connector, "connector", id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, connectorToJson(&data))
	if resp.Diagnostics.HasError() {
		// P0 may have staged the connector but returned it in a shape this can't use,
		// such as without its service account. It's in P0 but not in state, so remove it
		// and let the next apply stage it again.
		if _, err := r.installer.ProviderData.Delete(itemPath(installresources.Connector, id)); err != nil {
			resp.Diagnostics.AddWarning(
				"Could not remove the staged connector",
				fmt.Sprintf("P0 may still hold the connector %q in stage. If the next apply says it already exists, import it with `terraform import`. %s", id, err),
			)
		}
	}
}

func (r *JiraAiConnectorStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.reader.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Every settable attribute requires replacement, so Terraform never updates a staged
// connector in place. Staging it again would return an installed connector to
// "stage", so an update that gets here fails instead.
func (r *JiraAiConnectorStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Jira connectors can't be updated in place",
		"Every p0_jira_ai_connector_staged attribute requires replacement, so Terraform should never update one in place. Please report this as a provider bug.",
	)
}

func (r *JiraAiConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *JiraAiConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidConnectorId, isItemId, itemIdRule)
}
