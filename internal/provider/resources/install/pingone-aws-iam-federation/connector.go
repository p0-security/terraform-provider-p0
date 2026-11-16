package installpingoneawsiamfederation

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

// copy of internal/provider/resources/install/okta-aws-iam-federation/connector.go, renamed for
// PingOne (AWS IAM Federation) (INT-1473).

var _ resource.Resource = &PingOneAwsIamFederationConnector{}
var _ resource.ResourceWithConfigure = &PingOneAwsIamFederationConnector{}
var _ resource.ResourceWithImportState = &PingOneAwsIamFederationConnector{}
var _ resource.ResourceWithModifyPlan = &PingOneAwsIamFederationConnector{}
var _ resource.ResourceWithValidateConfig = &PingOneAwsIamFederationConnector{}

type PingOneAwsIamFederationConnector struct {
	installer *common.Install
}

type pingOneAwsIamFederationConnectorModel struct {
	Id                types.String  `tfsdk:"id"`
	Hosting           *hostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object  `tfsdk:"connector_identity"`
	State             types.String  `tfsdk:"state"`
}

// The staged and the final resource read the same item.
type pingOneAwsIamFederationConnectorJson struct {
	Hosting *hostingJson `json:"hosting,omitempty"`
	State   *string      `json:"state,omitempty"`
}

// What P0 computes for the item without storing it.
type pingOneAwsIamFederationConnectorMetadataJson struct {
	ConnectorIdentity *connectorIdentityJson `json:"connectorIdentity,omitempty"`
}

type pingOneAwsIamFederationConnectorApi struct {
	Item     *pingOneAwsIamFederationConnectorJson         `json:"item"`
	Metadata *pingOneAwsIamFederationConnectorMetadataJson `json:"metadata,omitempty"`
}

func (api *pingOneAwsIamFederationConnectorApi) identity() *connectorIdentityJson {
	if api.Metadata == nil {
		return nil
	}
	return api.Metadata.ConnectorIdentity
}

// Returns the whole response rather than the item alone, because the connector's
// identity is in the response's metadata, beside the item.
func getConnectorItemJson(json any) any {
	inner, ok := json.(*pingOneAwsIamFederationConnectorApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

// hosting is Required, so a nil hosting would surface as Terraform core's "provider
// produced inconsistent result after apply", which tells the reader nothing to do.
func requireHosting(diags *diag.Diagnostics, id string, item *pingOneAwsIamFederationConnectorJson) bool {
	if item.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The PingOne (AWS IAM Federation) connector %s carries no hosting configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return false
	}
	return true
}

func NewPingOneAwsIamFederationConnector() resource.Resource {
	return &PingOneAwsIamFederationConnector{}
}

func (*PingOneAwsIamFederationConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_pingone_aws_iam_federation_connector"
}

func (*PingOneAwsIamFederationConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An PingOne (AWS IAM Federation) connector installation.

The connector is how P0 assigns AWS roles in your PingOne SAML apps that sign users in to AWS. You deploy it in your own AWS account or Google Cloud project, and only it reads an app's client secret. One connector can serve many apps: register each with a ` + "`p0_pingone_aws_iam_federation_app`" + ` resource that names this connector.

**Important:** Before you create this resource, stage the installation with ` + "`p0_pingone_aws_iam_federation_connector_staged`" + ` and deploy the connector. Creating this resource has P0 check that it can invoke the connector, and on Google Cloud that it can read the Cloud Run service's URL. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the first check is denied access, apply again.

P0 fixes where a connector runs when it adds the connector, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces it, and the apps that name it must let the new connector's identity read their client secrets. If P0 hasn't finished installing the connector, for example because you imported it before its check passed, Terraform plans an update, and applying it finishes the install.

Destroying this resource returns the connector to its staged state, which ` + "`p0_pingone_aws_iam_federation_connector_staged`" + ` still owns. P0 can't grant or revoke access through a connector that isn't installed, so destroy the apps that name a connector before the connector.

**Prerequisites:**

- P0's PingOne (AWS IAM Federation) connector, deployed where ` + "`hosting`" + ` says and running as the identity that ` + "`connector_identity`" + ` names. P0's PingOne (AWS IAM Federation) installer generates Terraform that deploys the connector with its image pinned.

- For AWS Lambda, the image ` + "`p0security/p0-connector-pingone-aws-iam-federation-aws`" + `. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Install ` + "`p0_aws_iam_write`" + ` for the account the connector runs in, and grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.

- For Google Cloud Run, the image ` + "`p0security/p0-connector-pingone-aws-iam-federation-gcloud`" + `. Install ` + "`p0_gcp`" + `, and grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass the same address to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_pingone_aws_iam_federation_connector_staged` resource whose install this finishes",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting":            hostingAttribute(`Where the connector runs. Must match the ` + "`p0_pingone_aws_iam_federation_connector_staged`" + ` resource, whose ` + "`hosting`" + ` this attribute can take whole.`),
			"connector_identity": connectorIdentityAttribute(),
			"state":              common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*PingOneAwsIamFederationConnector) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(hostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *PingOneAwsIamFederationConnector) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        PingOneAwsIamFederationKey,
		Component:          installresources.Connector,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        getConnectorItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("connector"),
	}
}

func (r *PingOneAwsIamFederationConnector) getId(data any) *string {
	model, ok := data.(*pingOneAwsIamFederationConnectorModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *PingOneAwsIamFederationConnector) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*pingOneAwsIamFederationConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &pingOneAwsIamFederationConnectorModel{
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
func (r *PingOneAwsIamFederationConnector) toJson(data any) any {
	datav, ok := data.(*pingOneAwsIamFederationConnectorModel)
	if !ok {
		return nil
	}

	json := pingOneAwsIamFederationConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.toJson()
	}
	return &json
}

// The staged resource created the item, and P0 refuses a second PUT of a connector
// that's already added, so this only verifies and configures it.
func (r *PingOneAwsIamFederationConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pingOneAwsIamFederationConnectorApi{}, &pingOneAwsIamFederationConnectorModel{})
}

func (r *PingOneAwsIamFederationConnector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &pingOneAwsIamFederationConnectorApi{}, &pingOneAwsIamFederationConnectorModel{})
}

// Plans an update for a connector that P0 hasn't installed, so that the next apply
// finishes the install. On Google Cloud, P0 also looks up the Cloud Run service's URL
// during that apply, so the URL stays unknown until then. When the whole hosting block
// is unknown, the URL is unknown with it.
func (*PingOneAwsIamFederationConnector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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
func (r *PingOneAwsIamFederationConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pingOneAwsIamFederationConnectorApi{}, &pingOneAwsIamFederationConnectorModel{})
}

// Returns the item to the "stage" state rather than removing it, so that the staged
// resource still owns it. Other installs roll an item back with a PUT, which P0
// refuses for a connector that's already added. So this removes the item and stages it
// again where it was. If staging fails, the next apply finds the item gone and tries
// again.
func (r *PingOneAwsIamFederationConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &pingOneAwsIamFederationConnectorModel{})
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &pingOneAwsIamFederationConnectorModel{})
}

func (r *PingOneAwsIamFederationConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
