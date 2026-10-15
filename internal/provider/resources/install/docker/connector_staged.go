package installdocker

import (
	"context"

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

var _ resource.Resource = &DockerConnectorStaged{}
var _ resource.ResourceWithConfigure = &DockerConnectorStaged{}
var _ resource.ResourceWithImportState = &DockerConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &DockerConnectorStaged{}

type DockerConnectorStaged struct {
	installer *common.Install
}

type dockerConnectorStagedModel struct {
	Id                types.String        `tfsdk:"id"`
	Hosting           *stagedHostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object        `tfsdk:"connector_identity"`
	State             types.String        `tfsdk:"state"`
}

func NewDockerConnectorStaged() resource.Resource {
	return &DockerConnectorStaged{}
}

func (*DockerConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_docker_connector_staged"
}

func (*DockerConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Docker Hub connector installation. Staging tells P0 where the connector will run, and names the identity it must run as.

Use ` + "`hosting`" + ` and the read-only ` + "`connector_identity`" + ` to deploy P0's Docker Hub connector: a Lambda function and its IAM role on AWS, or a Cloud Run service and its service account on Google Cloud. Once the connector is deployed, create a ` + "`p0_docker_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation.

Where a connector runs can't change once it is added, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces the connector. If the connector is already added in P0, import it rather than creating it: P0 refuses to add a connector twice.

**Prerequisite:** P0 invokes the connector as its own identity in your cloud. For AWS Lambda, ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. For Google Cloud Run, ` + "`p0_gcp`" + ` must be installed.

**Note:** This integration is currently in beta.

For the full install steps, see [Docker Hub](https://docs.p0.dev/integrations/resource-integrations/docker).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A name you choose for the connector, such as ` + "`prod`" + `. Each organization picks its connector by it. Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters.`,
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting":            stagedHostingAttribute(`Where the connector runs. The connector's Terraform deploys it there, and P0 invokes it there.`),
			"connector_identity": connectorIdentityAttribute(),
			"state":              common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*DockerConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(stagedHostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *DockerConnectorStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  DockerKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  getConnectorItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *DockerConnectorStaged) getId(data any) *string {
	model, ok := data.(*dockerConnectorStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DockerConnectorStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*dockerConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &dockerConnectorStagedModel{
		Id:                types.StringValue(id),
		Hosting:           stagedHostingFromJson(jsonv.Item.Hosting),
		ConnectorIdentity: connectorIdentityFromJson(ctx, diags, jsonv.identity()),
		State:             types.StringPointerValue(jsonv.Item.State),
	}
}

// The body of the PUT that stages the item: where the connector runs. P0 names the
// connector's identity, and sets the state.
func (r *DockerConnectorStaged) toJson(data any) any {
	datav, ok := data.(*dockerConnectorStagedModel)
	if !ok {
		return nil
	}

	json := dockerConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.address().toJson()
	}
	return &json
}

func (r *DockerConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json dockerConnectorApi
	var data dockerConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *DockerConnectorStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &dockerConnectorApi{}, &dockerConnectorStagedModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update. P0 refuses a second PUT of a connector that's already added, so
// this doesn't stage the item again, as other staged resources' updates do. It refuses
// the update rather than report one that it didn't make.
func (r *DockerConnectorStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Docker Hub connectors can't be updated in place",
		"P0 fixes where a connector runs when it is added. Replace this resource to change it.",
	)
}

func (r *DockerConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &dockerConnectorStagedModel{})
}

func (r *DockerConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
