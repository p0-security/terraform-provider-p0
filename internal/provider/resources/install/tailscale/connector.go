package installtailscale

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// copy of internal/provider/resources/install/auth0/connector.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686).

var _ resource.Resource = &TailscaleConnector{}
var _ resource.ResourceWithConfigure = &TailscaleConnector{}
var _ resource.ResourceWithImportState = &TailscaleConnector{}
var _ resource.ResourceWithModifyPlan = &TailscaleConnector{}
var _ resource.ResourceWithValidateConfig = &TailscaleConnector{}

type TailscaleConnector struct {
	installer *common.Install
}

type tailscaleConnectorModel struct {
	Id                types.String  `tfsdk:"id"`
	Hosting           *hostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object  `tfsdk:"connector_identity"`
	State             types.String  `tfsdk:"state"`
}

// The staged and the final resource read the same item.
type tailscaleConnectorJson struct {
	Hosting *hostingJson `json:"hosting,omitempty"`
	State   *string      `json:"state,omitempty"`
}

// What P0 computes for the item without storing it.
type tailscaleConnectorMetadataJson struct {
	ConnectorIdentity *connectorIdentityJson `json:"connectorIdentity,omitempty"`
}

type tailscaleConnectorApi struct {
	Item     *tailscaleConnectorJson         `json:"item"`
	Metadata *tailscaleConnectorMetadataJson `json:"metadata,omitempty"`
}

func (api *tailscaleConnectorApi) identity() *connectorIdentityJson {
	if api.Metadata == nil {
		return nil
	}
	return api.Metadata.ConnectorIdentity
}

// Returns the whole response rather than the item alone, because the connector's
// identity is in the response's metadata, beside the item.
func getConnectorItemJson(json any) any {
	inner, ok := json.(*tailscaleConnectorApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

// hosting is Required, so a nil hosting would surface as Terraform core's "provider
// produced inconsistent result after apply", which tells the reader nothing to do.
func requireHosting(diags *diag.Diagnostics, id string, item *tailscaleConnectorJson) bool {
	if item.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Tailscale connector %s carries no hosting configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return false
	}
	return true
}

func NewTailscaleConnector() resource.Resource {
	return &TailscaleConnector{}
}

func (*TailscaleConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_tailscale_connector"
}

func (*TailscaleConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Tailscale connector installation.

The connector is how P0 grants network access on your tailnets. You deploy it in your own AWS account or Google Cloud project, where it calls only the Tailscale API, and only it holds a tailnet's Tailscale credential. One connector can serve many tailnets: register each with a ` + "`p0_tailscale_tailnet`" + ` resource that names this connector.

**Important:** Before you create this resource, stage the installation with ` + "`p0_tailscale_connector_staged`" + ` and deploy the connector. Creating this resource has P0 check that it can invoke the connector, and on Google Cloud that it can read the Cloud Run service's URL. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the first check is denied access, apply again.

P0 fixes where a connector runs when it adds the connector, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces it. Replacing it revokes nothing, and revokes on its tailnets fail until each tailnet's credential trusts the new connector's identity, so change ` + "`hosting`" + ` only when none of the connector's tailnets has active grants: revoke them first. If P0 hasn't finished installing the connector, for example because you imported it before its check passed, Terraform plans an update, and applying it finishes the install.

Destroying this resource returns the connector to its staged state, which ` + "`p0_tailscale_connector_staged`" + ` still owns. P0 can't grant or revoke access through a connector that isn't installed, so revoke the grants on the tailnets that name the connector, destroy those tailnets, then the connector. Destroying a tailnet revokes nothing.

**Prerequisites:**

- P0's Tailscale connector, deployed where ` + "`hosting`" + ` says and running as the identity that ` + "`connector_identity`" + ` names. P0's Tailscale installer generates Terraform that deploys the connector with its image pinned, as in the example.

- For AWS Lambda, the image ` + "`p0security/p0-connector-tailscale`" + `. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Install ` + "`p0_aws_iam_write`" + ` for the account the connector runs in, and grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function. For a tailnet with a federated identity, also turn on AWS outbound identity federation for the account, and let the connector's role call ` + "`sts:GetWebIdentityToken`" + ` for Tailscale's audiences, ` + "`api.tailscale.com/*`" + `, for 300 seconds or less.

- For Google Cloud Run, the image ` + "`p0security/p0-connector-tailscale-gcloud`" + `. Install ` + "`p0_gcp`" + `, and grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass the same address to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

**Note:** This integration is currently in beta.

For the full install steps, see [Tailscale](https://docs.p0.dev/integrations/resource-integrations/tailscale).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_tailscale_connector_staged` resource whose install this finishes",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting":            hostingAttribute(`Where the connector runs. Must match the ` + "`p0_tailscale_connector_staged`" + ` resource, whose ` + "`hosting`" + ` this attribute can take whole. ` + hostingChangeRevokesNothing),
			"connector_identity": connectorIdentityAttribute(),
			"state":              common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*TailscaleConnector) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(hostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *TailscaleConnector) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        TailscaleKey,
		Component:          installresources.Connector,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        getConnectorItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("connector"),
	}
}

func (r *TailscaleConnector) getId(data any) *string {
	model, ok := data.(*tailscaleConnectorModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *TailscaleConnector) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*tailscaleConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &tailscaleConnectorModel{
		Id:                types.StringValue(id),
		Hosting:           hostingFromJson(jsonv.Item.Hosting),
		ConnectorIdentity: connectorIdentityFromJson(ctx, diags, jsonv.identity()),
		State:             types.StringPointerValue(jsonv.Item.State),
	}
}

// The body of verify and configure, and of the PUT that stages the item again when
// this resource is destroyed: where the connector runs, which P0 fixed when it created
// the item. Sending it has P0 refuse a hosting that doesn't match the staged one,
// before it invokes anything. The Cloud Run URL stays out, because P0 looks it up.
func (r *TailscaleConnector) toJson(data any) any {
	datav, ok := data.(*tailscaleConnectorModel)
	if !ok {
		return nil
	}

	json := tailscaleConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.toJson()
	}
	return &json
}

// The staged resource created the item, and P0 refuses a second PUT of a connector
// that's already added, so this only verifies and configures it.
func (r *TailscaleConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &tailscaleConnectorApi{}, &tailscaleConnectorModel{})
}

func (r *TailscaleConnector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &tailscaleConnectorApi{}, &tailscaleConnectorModel{})
}

// Plans an update for a connector that P0 hasn't installed, so that the next apply
// finishes the install. On Google Cloud, P0 also looks up the Cloud Run service's URL
// during that apply, so the URL stays unknown until then. When the whole hosting block
// is unknown, the URL is unknown with it.
func (*TailscaleConnector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does.
func (r *TailscaleConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &tailscaleConnectorApi{}, &tailscaleConnectorModel{})
}

// Returns the item to the "stage" state rather than removing it, so that the staged
// resource still owns it. Other installs roll an item back with a PUT, which P0
// refuses for a connector that's already added. So this removes the item and stages it
// again where it was. If staging fails, the next apply finds the item gone and tries
// again.
func (r *TailscaleConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &tailscaleConnectorModel{})
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &tailscaleConnectorModel{})
}

func (r *TailscaleConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
