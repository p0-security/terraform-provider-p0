package installsharepointai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &SharePointConnector{}
var _ resource.ResourceWithConfigure = &SharePointConnector{}
var _ resource.ResourceWithImportState = &SharePointConnector{}

type SharePointConnector struct {
	installer *common.Install
}

type sharePointConnectorModel struct {
	DirectoryId            types.String `tfsdk:"directory_id"`
	ConnectorClientId      types.String `tfsdk:"connector_client_id"`
	ConnectorName          types.String `tfsdk:"connector_name"`
	ContainerAppResourceId types.String `tfsdk:"container_app_resource_id"`
	ConnectorServiceUri    types.String `tfsdk:"connector_service_uri"`
	State                  types.String `tfsdk:"state"`
}

type sharePointConnectorJson struct {
	ConnectorClientId      *string `json:"connectorClientId,omitempty"`
	ConnectorName          *string `json:"connectorName,omitempty"`
	ContainerAppResourceId *string `json:"containerAppResourceId,omitempty"`
	ConnectorServiceUri    *string `json:"connectorServiceUri,omitempty"`
	State                  *string `json:"state,omitempty"`
}

type sharePointConnectorApi struct {
	Item *sharePointConnectorJson `json:"item"`
}

func NewSharePointConnector() resource.Resource {
	return &SharePointConnector{}
}

func (*SharePointConnector) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_sharepoint_connector"
}

func (*SharePointConnector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A SharePoint connector installation for a Microsoft Entra directory.

Installing the SharePoint connector lets P0 grant the app registrations the Entra Agent Identity Creator makes for agentic grants access to SharePoint sites in this directory.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_sharepoint_connector_staged`" + ` and deploy the connector's Container App and app registration. Creating this resource reads the Container App as that app registration, checks that the connector trusts the Entra Agent Identity Creator's identity, and checks that P0 can reach the connector. A new federated identity credential can take a few minutes to start working, so the first apply can take several minutes.

` + previewNote + `

` + docsLink,
		Attributes: map[string]schema.Attribute{
			"directory_id": directoryIdAttribute(`The ` + "`directory_id`" + ` of the ` + "`p0_sharepoint_connector_staged`" + ` resource being finalized`),
			"connector_client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID, in lowercase, of the connector's app registration, which P0 calls the connector as. It is the Terraform output ` + "`p0_sharepoint_ai_connector_client_id`" + ` of the Terraform P0 shows.`,
				Validators:          []validator.String{guidValidator("connector client ID")},
			},
			"connector_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name of the connector's Container App`,
			},
			"container_app_resource_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The resource ID of the connector's Container App`,
			},
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The connector's URL, read from its Container App`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *SharePointConnector) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:           SharePointAiKey,
		Component:             installresources.Connector,
		ProviderData:          data,
		GetId:                 r.getId,
		GetItemJson:           r.getItemJson,
		FromJson:              r.fromJson,
		ToJson:                r.toJson,
		DescribeWriteNotFound: describeNotOffered,
	}
}

// The item is keyed by the directory.
func (r *SharePointConnector) getId(data any) *string {
	model, ok := data.(*sharePointConnectorModel)
	if !ok {
		return nil
	}
	str := model.DirectoryId.ValueString()
	return &str
}

func (r *SharePointConnector) getItemJson(json any) any {
	inner, ok := json.(*sharePointConnectorApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SharePointConnector) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*sharePointConnectorJson)
	if !ok {
		return nil
	}

	return &sharePointConnectorModel{
		DirectoryId:            types.StringValue(id),
		ConnectorClientId:      types.StringPointerValue(jsonv.ConnectorClientId),
		ConnectorName:          types.StringPointerValue(jsonv.ConnectorName),
		ContainerAppResourceId: types.StringPointerValue(jsonv.ContainerAppResourceId),
		ConnectorServiceUri:    types.StringPointerValue(jsonv.ConnectorServiceUri),
		State:                  types.StringPointerValue(jsonv.State),
	}
}

// Sends only the connector's client ID: P0 generates every other field.
func (r *SharePointConnector) toJson(data any) any {
	datav, ok := data.(*sharePointConnectorModel)
	if !ok {
		return nil
	}
	return &sharePointConnectorJson{
		ConnectorClientId: datav.ConnectorClientId.ValueStringPointer(),
	}
}

func (r *SharePointConnector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json sharePointConnectorApi
	var data sharePointConnectorModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *SharePointConnector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sharePointConnectorApi{}, &sharePointConnectorModel{})
}

// P0 runs every install check on configure, so an update posts to configure alone: a
// refused client ID then leaves the installed item, and this resource's state, as they
// were.
func (r *SharePointConnector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &sharePointConnectorApi{}, &sharePointConnectorModel{})
}

// Returns the item to the "stage" state rather than deleting it, so that the staged
// resource still owns it.
func (r *SharePointConnector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &sharePointConnectorModel{})
}

func (r *SharePointConnector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("directory_id"), req, resp)
}
