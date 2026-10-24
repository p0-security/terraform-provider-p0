package installjiraai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &JiraAiConnector{}
var _ resource.ResourceWithConfigure = &JiraAiConnector{}
var _ resource.ResourceWithImportState = &JiraAiConnector{}
var _ resource.ResourceWithModifyPlan = &JiraAiConnector{}

// A Google Cloud-only copy of Harbor's p0_harbor_connector (harbor/connector.go).
type JiraAiConnector struct {
	installer *common.Install
	reader    *common.Install
}

func NewJiraAiConnector() resource.Resource {
	return &JiraAiConnector{}
}

func (*JiraAiConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_jira_ai_connector"
}

func (*JiraAiConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Jira connector installation, for Jira agent access.

The connector is how P0 reaches your Jira Cloud sites. You deploy it in your own Google Cloud project, and one connector can serve several sites. It reads each site's admin client secret and agent account API keys from Google Secret Manager as its own service account, so P0 never holds them. Once it is installed, add each site with ` + "`p0_jira_ai_site`" + `.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_jira_ai_connector_staged`" + ` and deploy the connector. Creating this resource has P0 find the Cloud Run service, check that it runs as ` + "`hosting.connector_service_account`" + `, and check that P0 can invoke it. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the first check is denied access, apply again.

` + "`id`" + ` and ` + "`hosting`" + ` must match the staged resource, and changing either replaces the connector. A connector that P0 hasn't finished installing, such as one imported before its check passed, plans an update, and applying it finishes the install.

Destroying this resource returns the connector to its staged state, which ` + "`p0_jira_ai_connector_staged`" + ` still owns. P0 can't reach a site through a connector that isn't installed, so destroy the sites that name a connector before the connector. Terraform does so in the same apply when the site refers to this resource, as the examples do.

**Prerequisites:**

- P0's Jira connector image, ` + "`p0security/p0-connector-jira-ai-gcloud`" + `, deployed as a Cloud Run service where ` + "`hosting`" + ` says, running as the staged resource's ` + "`hosting.connector_service_account`" + `. P0's Jira installer generates Terraform that deploys the connector with its image pinned, as in the example.

- ` + "`p0_gcp`" + ` installed. Grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass the same address to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

` + betaNote,
		Attributes: connectorAttributes(
			"The `id` of the `p0_jira_ai_connector_staged` resource being finalized",
			`Where the connector runs. Must match the `+"`p0_jira_ai_connector_staged`"+` resource.`,
		),
	}
}

func (r *JiraAiConnector) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = newConnectorInstaller(data)
	r.reader = newConnectorReader(data)
}

// The staged resource created the item, so this only verifies and configures it. It
// doesn't PUT first: P0 doesn't refuse a PUT that changes where a connector runs, so a
// hosting that differs from the staged one would silently move the connector. Every
// hosting attribute forces replacement, so a plan never reaches this with a changed hosting.
func (r *JiraAiConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *JiraAiConnector) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.reader.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Plans an update for a connector that P0 hasn't installed, so that the next apply
// finishes the install. That is also when P0 looks up the Cloud Run service's URL, so
// the URL is unknown until then, unless the whole hosting block is.
func (*JiraAiConnector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var hosting types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("hosting"), &hosting)...)
	if resp.Diagnostics.HasError() {
		return
	}

	computed := []path.Path{}
	if !hosting.IsNull() && !hosting.IsUnknown() {
		computed = append(computed, path.Root("hosting").AtName("connector_service_uri"))
	}
	planUnfinishedInstall(ctx, req, resp, computed...)
}

// Every settable attribute requires replacement, so the only update Terraform plans
// is the one that finishes an install.
func (r *JiraAiConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	finishInstall(ctx, r.installer, req, resp, &connectorApi{}, &connectorModel{})
}

// Returns the item to "stage" rather than deleting it, so the staged resource still
// owns it. The PUT sends the hosting P0 already has.
func (r *JiraAiConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *JiraAiConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidConnectorId, isItemId, itemIdRule)
}
