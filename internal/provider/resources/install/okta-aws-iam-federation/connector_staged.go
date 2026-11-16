package installoktaawsiamfederation

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

// copy of internal/provider/resources/install/auth0/connector_staged.go, renamed for Okta (AWS IAM
// Federation) (INT-1473).

var _ resource.Resource = &OktaAwsIamFederationConnectorStaged{}
var _ resource.ResourceWithConfigure = &OktaAwsIamFederationConnectorStaged{}
var _ resource.ResourceWithImportState = &OktaAwsIamFederationConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &OktaAwsIamFederationConnectorStaged{}

type OktaAwsIamFederationConnectorStaged struct {
	installer *common.Install
}

type oktaAwsIamFederationConnectorStagedModel struct {
	Id                types.String        `tfsdk:"id"`
	Hosting           *stagedHostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object        `tfsdk:"connector_identity"`
	State             types.String        `tfsdk:"state"`
}

func NewOktaAwsIamFederationConnectorStaged() resource.Resource {
	return &OktaAwsIamFederationConnectorStaged{}
}

func (*OktaAwsIamFederationConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_okta_aws_iam_federation_connector_staged"
}

func (*OktaAwsIamFederationConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Okta (AWS IAM Federation) connector installation. Staging tells P0 where the connector will run, and names the identity it must run as.

Use ` + "`hosting`" + ` and the read-only ` + "`connector_identity`" + ` to deploy P0's Okta (AWS IAM Federation) connector: a Lambda function and its IAM role on AWS, or a Cloud Run service and its service account on Google Cloud. Once the connector is deployed, create a ` + "`p0_okta_aws_iam_federation_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation.

P0 fixes where a connector runs when it adds the connector, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces it. If the connector is already added in P0, import it rather than creating it: P0 refuses to add a connector twice.

**Prerequisite:** P0 invokes the connector as its own identity in your cloud. For AWS Lambda, install ` + "`p0_aws_iam_write`" + ` for the account the connector runs in. For Google Cloud Run, install ` + "`p0_gcp`" + `.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A name you choose for the connector, such as ` + "`prod`" + `. Each app picks its connector by it. Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters.`,
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

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*OktaAwsIamFederationConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(stagedHostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *OktaAwsIamFederationConnectorStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  OktaAwsIamFederationKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  getConnectorItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *OktaAwsIamFederationConnectorStaged) getId(data any) *string {
	model, ok := data.(*oktaAwsIamFederationConnectorStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *OktaAwsIamFederationConnectorStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*oktaAwsIamFederationConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &oktaAwsIamFederationConnectorStagedModel{
		Id:                types.StringValue(id),
		Hosting:           stagedHostingFromJson(jsonv.Item.Hosting),
		ConnectorIdentity: connectorIdentityFromJson(ctx, diags, jsonv.identity()),
		State:             types.StringPointerValue(jsonv.Item.State),
	}
}

// The body of the PUT that stages the item: where the connector runs. P0 names the
// connector's identity, and sets the state.
func (r *OktaAwsIamFederationConnectorStaged) toJson(data any) any {
	datav, ok := data.(*oktaAwsIamFederationConnectorStagedModel)
	if !ok {
		return nil
	}

	json := oktaAwsIamFederationConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.address().toJson()
	}
	return &json
}

func (r *OktaAwsIamFederationConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaAwsIamFederationConnectorApi
	var data oktaAwsIamFederationConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *OktaAwsIamFederationConnectorStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &oktaAwsIamFederationConnectorApi{}, &oktaAwsIamFederationConnectorStagedModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update. Other staged resources' updates stage the item again, but P0
// refuses a second PUT of a connector that's already added. So this fails the update
// rather than report one it didn't make.
func (r *OktaAwsIamFederationConnectorStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Okta (AWS IAM Federation) connectors can't be updated in place",
		"P0 fixes where a connector runs when it adds the connector. Replace this resource to change it.",
	)
}

func (r *OktaAwsIamFederationConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &oktaAwsIamFederationConnectorStagedModel{})
}

func (r *OktaAwsIamFederationConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
