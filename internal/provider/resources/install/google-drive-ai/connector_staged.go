package installgoogledriveai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

var _ resource.Resource = &GoogleDriveConnectorStaged{}
var _ resource.ResourceWithConfigure = &GoogleDriveConnectorStaged{}
var _ resource.ResourceWithImportState = &GoogleDriveConnectorStaged{}

type GoogleDriveConnectorStaged struct {
	installer *common.Install
}

type googleDriveConnectorStagedModel struct {
	ProjectId               types.String `tfsdk:"project_id"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	State                   types.String `tfsdk:"state"`
}

type googleDriveConnectorStagedJson struct {
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type googleDriveConnectorStagedApi struct {
	Item *googleDriveConnectorStagedJson `json:"item"`
}

func NewGoogleDriveConnectorStaged() resource.Resource {
	return &GoogleDriveConnectorStaged{}
}

func (*GoogleDriveConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_google_drive_connector_staged"
}

func (*GoogleDriveConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Google Drive connector installation for a Google Cloud project. Staging generates the identifiers needed to deploy P0's Google Drive connector in that project.

Use the read-only ` + "`connector_service_name`" + `, ` + "`connector_service_account`" + ` and ` + "`region`" + ` attributes to deploy the connector's Cloud Run service and its service account. Once the connector is deployed, create a ` + "`p0_google_drive_connector`" + ` resource with the same ` + "`project_id`" + ` to complete the installation.

**Prerequisite:** the GCP Service Account Creator (` + "`p0_gcp_service_account_creator`" + `) must be installed for the same project.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Google Cloud project to run the connector in. P0 also creates the identities it grants Drive access to in this project, so the GCP Service Account Creator must be installed for it.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The Google Cloud region to deploy the connector's Cloud Run service in`,
			},
			"connector_service_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name to give the connector's Cloud Run service`,
			},
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the service account the connector runs as. Add it as a Manager on each shared drive P0 grants access in.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GoogleDriveConnectorStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GoogleDriveAiKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

// The item is keyed by the project.
func (r *GoogleDriveConnectorStaged) getId(data any) *string {
	model, ok := data.(*googleDriveConnectorStagedModel)
	if !ok {
		return nil
	}
	str := model.ProjectId.ValueString()
	return &str
}

func (r *GoogleDriveConnectorStaged) getItemJson(json any) any {
	inner, ok := json.(*googleDriveConnectorStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GoogleDriveConnectorStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := googleDriveConnectorStagedModel{}
	jsonv, ok := json.(*googleDriveConnectorStagedJson)
	if !ok {
		return nil
	}

	data.ProjectId = types.StringValue(id)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)
	data.State = types.StringPointerValue(jsonv.State)

	return &data
}

func (r *GoogleDriveConnectorStaged) toJson(data any) any {
	if _, ok := data.(*googleDriveConnectorStagedModel); !ok {
		return nil
	}
	// The project is the item id, and P0 assigns every other field, so the body is
	// empty.
	return &googleDriveConnectorStagedJson{}
}

func (r *GoogleDriveConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json googleDriveConnectorStagedApi
	var data googleDriveConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *GoogleDriveConnectorStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &googleDriveConnectorStagedApi{}, &googleDriveConnectorStagedModel{})
}

func (r *GoogleDriveConnectorStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json googleDriveConnectorStagedApi
	var data googleDriveConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *GoogleDriveConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &googleDriveConnectorStagedModel{})
}

func (r *GoogleDriveConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
