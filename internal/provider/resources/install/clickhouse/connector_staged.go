package installclickhouse

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

// copy of internal/provider/resources/install/auth0/connector_staged.go (#245),
// itself a copy of Docker Hub's, renamed for ClickHouse Cloud (INT-695).

var _ resource.Resource = &ClickHouseConnectorStaged{}
var _ resource.ResourceWithConfigure = &ClickHouseConnectorStaged{}
var _ resource.ResourceWithImportState = &ClickHouseConnectorStaged{}
var _ resource.ResourceWithValidateConfig = &ClickHouseConnectorStaged{}

type ClickHouseConnectorStaged struct {
	installer *common.Install
}

type clickhouseConnectorStagedModel struct {
	Id                types.String        `tfsdk:"id"`
	Hosting           *stagedHostingModel `tfsdk:"hosting"`
	ConnectorIdentity types.Object        `tfsdk:"connector_identity"`
	State             types.String        `tfsdk:"state"`
}

func NewClickHouseConnectorStaged() resource.Resource {
	return &ClickHouseConnectorStaged{}
}

func (*ClickHouseConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_clickhouse_connector_staged"
}

func (*ClickHouseConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged ClickHouse Cloud connector installation. Staging tells P0 where the connector will run, and names the identity it must run as.

Use ` + "`hosting`" + ` and the read-only ` + "`connector_identity`" + ` to deploy P0's ClickHouse Cloud connector: a Lambda function and its IAM role on AWS, or a Cloud Run service and its service account on Google Cloud. Once the connector is deployed, create a ` + "`p0_clickhouse_connector`" + ` resource with the same ` + "`id`" + ` and ` + "`hosting`" + ` to complete the installation.

P0 fixes where a connector runs when it adds the connector, so changing ` + "`id`" + ` or ` + "`hosting`" + ` replaces it. If the connector is already added in P0, import it rather than creating it: P0 refuses to add a connector twice.

**Prerequisite:** P0 invokes the connector as its own identity in your cloud. For AWS Lambda, install ` + "`p0_aws_iam_write`" + ` for the account the connector runs in. For Google Cloud Run, install ` + "`p0_gcp`" + `.

**Note:** This integration is currently in beta.

For the full install steps, see [ClickHouse Cloud](https://docs.p0.dev/integrations/resource-integrations/clickhouse).`,
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

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*ClickHouseConnectorStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateHosting(stagedHostingFromConfig(ctx, req.Config, &resp.Diagnostics), &resp.Diagnostics)
}

func (r *ClickHouseConnectorStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  ClickHouseKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  getConnectorItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *ClickHouseConnectorStaged) getId(data any) *string {
	model, ok := data.(*clickhouseConnectorStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *ClickHouseConnectorStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*clickhouseConnectorApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	if !requireHosting(diags, id, jsonv.Item) {
		return nil
	}

	return &clickhouseConnectorStagedModel{
		Id:                types.StringValue(id),
		Hosting:           stagedHostingFromJson(jsonv.Item.Hosting),
		ConnectorIdentity: connectorIdentityFromJson(ctx, diags, jsonv.identity()),
		State:             types.StringPointerValue(jsonv.Item.State),
	}
}

// The body of the PUT that stages the item: where the connector runs. P0 names the
// connector's identity, and sets the state.
func (r *ClickHouseConnectorStaged) toJson(data any) any {
	datav, ok := data.(*clickhouseConnectorStagedModel)
	if !ok {
		return nil
	}

	json := clickhouseConnectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.address().toJson()
	}
	return &json
}

func (r *ClickHouseConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json clickhouseConnectorApi
	var data clickhouseConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *ClickHouseConnectorStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &clickhouseConnectorApi{}, &clickhouseConnectorStagedModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update. Other staged resources' updates stage the item again, but P0
// refuses a second PUT of a connector that's already added. So this fails the update
// rather than report one it didn't make.
func (r *ClickHouseConnectorStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"ClickHouse Cloud connectors can't be updated in place",
		"P0 fixes where a connector runs when it adds the connector. Replace this resource to change it.",
	)
}

func (r *ClickHouseConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &clickhouseConnectorStagedModel{})
}

func (r *ClickHouseConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
