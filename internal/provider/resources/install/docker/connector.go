package installdocker

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

var _ resource.Resource = &DockerConnector{}
var _ resource.ResourceWithConfigure = &DockerConnector{}
var _ resource.ResourceWithImportState = &DockerConnector{}
var _ resource.ResourceWithModifyPlan = &DockerConnector{}
var _ resource.ResourceWithValidateConfig = &DockerConnector{}

type DockerConnector struct {
	installer *common.Install
}

type dockerConnectorModel struct {
	Id                types.String  `tfsdk:"id"`
	Hosting           *hostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object  `tfsdk:"connector_identity"`
	State             types.String  `tfsdk:"state"`
}

// The item, as P0 stores it. The staged and the final resource read the same item.
type dockerConnectorJson struct {
	Hosting *hostingJson `json:"hosting,omitempty"`
	State   *string      `json:"state,omitempty"`
}

// What P0 computes for the item without storing it.
type dockerConnectorMetadataJson struct {
	ConnectorIdentity *connectorIdentityJson `json:"connectorIdentity,omitempty"`
}

type dockerConnectorApi struct {
	Item     *dockerConnectorJson         `json:"item"`
	Metadata *dockerConnectorMetadataJson `json:"metadata,omitempty"`
}

func (api *dockerConnectorApi) identity() *connectorIdentityJson {
	if api.Metadata == nil {
		return nil
	}
	return api.Metadata.ConnectorIdentity
}

// The connector's identity is in the response's metadata, beside the item, so the
// whole response is read rather than the item alone.
func getConnectorItemJson(json any) any {
	inner, ok := json.(*dockerConnectorApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

// hosting is Required, so leaving it nil would surface as Terraform core's "provider
// produced inconsistent result after apply" rather than something the reader can act
// on.
func requireHosting(diags *diag.Diagnostics, id string, item *dockerConnectorJson) bool {
	if item.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Docker Hub connector %s carries no hosting configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return false
	}
	return true
}

func NewDockerConnector() resource.Resource {
	return &DockerConnector{}
}

func (*DockerConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_docker_connector"
}

func (*DockerConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Docker Hub connector installation.

The connector is how P0 grants roles in your Docker Hub organizations. You deploy it in your own AWS account or Google Cloud project, and only it reads an organization's access token. It acts only for the email domains its deployment allows, in its ` + "`DOMAIN_ALLOW_PATTERN`" + `. One connector can serve many organizations: register each with a ` + "`p0_docker_organization`" + ` resource that names this connector.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_docker_connector_staged`" + ` and deploy the connector. Creating this resource has P0 check that it can invoke the connector, and on Google Cloud that it can read the Cloud Run service's URL. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

Where a connector runs can't change once it is added, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces the connector, and the organizations that name it must let the new connector's identity read their access tokens. A connector that P0 hasn't finished installing, such as one imported before its check passed, plans an update, and applying it finishes the install.

Destroying this resource returns the connector to its staged state, which ` + "`p0_docker_connector_staged`" + ` still owns. P0 can't grant or revoke roles through a connector that isn't installed, so destroy the organizations that name a connector before the connector.

**Prerequisites:**

- P0's Docker Hub connector, deployed where ` + "`hosting`" + ` says and running as the identity that ` + "`connector_identity`" + ` names. Its ` + "`DOMAIN_ALLOW_PATTERN`" + ` environment variable is a regular expression that the connector tests against the part of an email after the ` + "`@`" + `, and the connector won't start without it. Nothing anchors the pattern, so write it anchored, such as ` + "`^example\\.com$`" + `. P0's Docker Hub installer generates Terraform that deploys the connector with its image pinned, as in the example.

- For AWS Lambda, the image ` + "`p0security/p0-connector-docker`" + `. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. Grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.

- For Google Cloud Run, the image ` + "`p0security/p0-connector-docker-gcloud`" + `. ` + "`p0_gcp`" + ` must be installed. Grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass the same address to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_docker_connector_staged` resource being finalized",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting":            hostingAttribute(`Where the connector runs. Must match the ` + "`p0_docker_connector_staged`" + ` resource, whose ` + "`hosting`" + ` this attribute can take whole.`),
			"connector_identity": connectorIdentityAttribute(),
			"state":              common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*DockerConnector) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(hostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *DockerConnector) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        DockerKey,
		Component:          installresources.Connector,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        getConnectorItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("connector"),
	}
}

func (r *DockerConnector) getId(data any) *string {
	model, ok := data.(*dockerConnectorModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DockerConnector) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*dockerConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &dockerConnectorModel{
		Id:                types.StringValue(id),
		Hosting:           hostingFromJson(jsonv.Item.Hosting),
		ConnectorIdentity: connectorIdentityFromJson(ctx, diags, jsonv.identity()),
		State:             types.StringPointerValue(jsonv.Item.State),
	}
}

// The body of verify and configure, and of the PUT that stages the item again when
// this resource is destroyed: where the connector runs, which P0 fixed when it created
// the item. Sending it has P0 refuse a hosting that doesn't match the staged one,
// before it invokes anything. The Cloud Run URL is left out, because P0 resolves it.
func (r *DockerConnector) toJson(data any) any {
	datav, ok := data.(*dockerConnectorModel)
	if !ok {
		return nil
	}

	json := dockerConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.toJson()
	}
	return &json
}

// The staged resource created the item, and P0 refuses a second PUT of a connector
// that's already added, so this only verifies and configures it.
func (r *DockerConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &dockerConnectorApi{}, &dockerConnectorModel{})
}

func (r *DockerConnector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &dockerConnectorApi{}, &dockerConnectorModel{})
}

// Plans an update for a connector that P0 hasn't installed, so that the next apply
// finishes the install. On Google Cloud that is also when P0 resolves the Cloud Run
// service's URL, so the URL is unknown until then, unless the whole hosting block is.
func (*DockerConnector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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
func (r *DockerConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &dockerConnectorApi{}, &dockerConnectorModel{})
}

// Returns the item to the "stage" state rather than removing it, so that the staged
// resource still owns it. Other installs roll an item back with a PUT, which P0
// refuses for a connector that's already added. So this removes the item and stages it
// again, where it was. If staging fails, the next apply finds the item gone and tries
// again.
func (r *DockerConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &dockerConnectorModel{})
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &dockerConnectorModel{})
}

func (r *DockerConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
