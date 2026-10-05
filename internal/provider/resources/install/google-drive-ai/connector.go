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

var _ resource.Resource = &GoogleDriveConnector{}
var _ resource.ResourceWithConfigure = &GoogleDriveConnector{}
var _ resource.ResourceWithImportState = &GoogleDriveConnector{}

type GoogleDriveConnector struct {
	installer *common.Install
}

type googleDriveConnectorModel struct {
	ProjectId               types.String `tfsdk:"project_id"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	ConnectorServiceUri     types.String `tfsdk:"connector_service_uri"`
	State                   types.String `tfsdk:"state"`
}

type googleDriveConnectorJson struct {
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type googleDriveConnectorApi struct {
	Item *googleDriveConnectorJson `json:"item"`
}

func NewGoogleDriveConnector() resource.Resource {
	return &GoogleDriveConnector{}
}

func (*GoogleDriveConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_google_drive_connector"
}

func (*GoogleDriveConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Drive connector installation for a Google Cloud project.

The connector is how P0 grants agentic sessions access to folders and files in Google shared drives. Register each shared drive with a ` + "`p0_google_drive_shared_drive`" + ` resource that names this connector's project.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_google_drive_connector_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector's Cloud Run service is deployed.

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
				MarkdownDescription: `The Google Cloud region the connector's Cloud Run service runs in`,
			},
			"connector_service_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name of the connector's Cloud Run service`,
			},
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the service account the connector runs as. Add it as a Manager on each shared drive P0 grants access in.`,
			},
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The invocation URL of the connector's Cloud Run service`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GoogleDriveConnector) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *GoogleDriveConnector) getId(data any) *string {
	model, ok := data.(*googleDriveConnectorModel)
	if !ok {
		return nil
	}
	str := model.ProjectId.ValueString()
	return &str
}

func (r *GoogleDriveConnector) getItemJson(json any) any {
	inner, ok := json.(*googleDriveConnectorApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GoogleDriveConnector) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := googleDriveConnectorModel{}
	jsonv, ok := json.(*googleDriveConnectorJson)
	if !ok {
		return nil
	}

	data.ProjectId = types.StringValue(id)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)
	data.ConnectorServiceUri = types.StringPointerValue(jsonv.ConnectorServiceUri)
	data.State = types.StringPointerValue(jsonv.State)

	return &data
}

func (r *GoogleDriveConnector) toJson(data any) any {
	if _, ok := data.(*googleDriveConnectorModel); !ok {
		return nil
	}
	// The project is the item id, and P0 assigns every other field, so the body is
	// empty.
	return &googleDriveConnectorJson{}
}

func (r *GoogleDriveConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json googleDriveConnectorApi
	var data googleDriveConnectorModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GoogleDriveConnector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &googleDriveConnectorApi{}, &googleDriveConnectorModel{})
}

func (r *GoogleDriveConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &googleDriveConnectorApi{}, &googleDriveConnectorModel{})
}

// Returns the item to the "stage" state rather than deleting it, so that the staged
// resource still owns it.
func (r *GoogleDriveConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &googleDriveConnectorModel{})
}

func (r *GoogleDriveConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
