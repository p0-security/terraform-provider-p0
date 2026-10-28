package installlaunchdarkly

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &LaunchDarklyConnector{}
var _ resource.ResourceWithConfigure = &LaunchDarklyConnector{}
var _ resource.ResourceWithImportState = &LaunchDarklyConnector{}
var _ resource.ResourceWithValidateConfig = &LaunchDarklyConnector{}

type LaunchDarklyConnector struct {
	installer *common.Install
	reader    *common.Install
}

func NewLaunchDarklyConnector() resource.Resource {
	return &LaunchDarklyConnector{}
}

func (*LaunchDarklyConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_launchdarkly_connector"
}

func (*LaunchDarklyConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A LaunchDarkly connector installation.

The connector is how P0 reaches your LaunchDarkly accounts. You deploy it in your own AWS account or Google Cloud project, and one connector can serve several LaunchDarkly accounts. Once it is installed, add each account with ` + "`p0_launchdarkly_account`" + `.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_launchdarkly_connector_staged`" + ` and deploy the connector. Creating this resource has P0 check the connector where ` + "`hosting`" + ` says it runs. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

` + "`id`" + ` and ` + "`hosting`" + ` must match the staged resource. P0 refuses to install a connector anywhere but where it was staged, and changing either replaces the connector.

Destroying this resource returns the connector to its staged state, which ` + "`p0_launchdarkly_connector_staged`" + ` still owns. P0 refuses that while a LaunchDarkly account uses the connector, so remove each ` + "`p0_launchdarkly_account`" + ` that uses it first. Terraform does so in the same apply when the account refers to this resource, as the examples do.

**Prerequisites:**

- P0's LaunchDarkly connector, deployed where ` + "`hosting`" + ` says. Set its ` + "`DOMAIN_ALLOW_PATTERN`" + ` environment variable to a regular expression that an email's whole domain must match for its member to receive access, such as ` + "`example[.]com`" + `. The pattern is set on the connector alone, and isn't an attribute of this resource: P0 doesn't set it.

- For AWS Lambda, the image ` + "`p0security/p0-connector-launchdarkly`" + `. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. Grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.

- For Google Cloud Run, the image ` + "`p0security/p0-connector-launchdarkly-gcloud`" + `, running as the staged resource's ` + "`hosting.connector_service_account`" + `. ` + "`p0_gcp`" + ` must be installed. Grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass the same address to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

` + previewNote + `

` + installDocs,
		Attributes: connectorAttributes(
			"The `id` of the `p0_launchdarkly_connector_staged` resource being finalized",
			`Where the connector runs. Must match the `+"`p0_launchdarkly_connector_staged`"+` resource.`,
		),
	}
}

func (*LaunchDarklyConnector) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateConnector(ctx, req.Config, &resp.Diagnostics)
}

func (r *LaunchDarklyConnector) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = newConnectorInstaller(data)
	r.reader = newConnectorReader(data)
}

// The staged resource created the item, so this only verifies and configures it. It
// doesn't stage the item first, as other final resources do: a PUT skips P0's refusal
// to change where a connector runs, so a hosting that doesn't match the staged one
// would move the connector without a word. On verify, P0 refuses it.
func (r *LaunchDarklyConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

func (r *LaunchDarklyConnector) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.reader.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
}

// Every attribute that can be set requires replacement, so Terraform never plans an
// update. One would verify and configure the item as Create does.
func (r *LaunchDarklyConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
}

// Returns the item to the "stage" state rather than deleting it, so that the staged
// resource still owns it. The PUT that does so sends the hosting P0 already has, which
// P0 accepts for an item that exists, unless a LaunchDarkly account uses the
// connector: then P0 refuses to stage it again (409), since every such account would
// stop working. This is common.Install's Rollback, with that refusal told apart.
func (r *LaunchDarklyConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data connectorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.Id.ValueString()
	httpResp, err := r.installer.ProviderData.Put(itemPath(installresources.Connector, id), connectorToJson(&data), &struct{}{})
	switch {
	case err == nil:
	case httpResp != nil && httpResp.StatusCode == http.StatusConflict:
		resp.Diagnostics.AddError(
			"LaunchDarkly connector is in use",
			fmt.Sprintf("P0 won't return the LaunchDarkly connector %q to its staged state while a LaunchDarkly account uses it. "+
				"Remove each p0_launchdarkly_account that uses it first, in the same apply or an earlier one, then apply again.\n\nP0's response: %s", id, err),
		)
	default:
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Could not return the LaunchDarkly connector %q to its staged state, got error:\n%s", id, err))
	}
}

func (r *LaunchDarklyConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidConnectorId)
}
