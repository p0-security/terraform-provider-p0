package installsharepointai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &SharePointConnectorStaged{}
var _ resource.ResourceWithConfigure = &SharePointConnectorStaged{}
var _ resource.ResourceWithImportState = &SharePointConnectorStaged{}

type SharePointConnectorStaged struct {
	installer *common.Install
}

type sharePointConnectorStagedModel struct {
	DirectoryId            types.String `tfsdk:"directory_id"`
	ConnectorName          types.String `tfsdk:"connector_name"`
	ContainerAppResourceId types.String `tfsdk:"container_app_resource_id"`
	State                  types.String `tfsdk:"state"`
}

type sharePointConnectorStagedJson struct {
	ConnectorName          *string `json:"connectorName,omitempty"`
	ContainerAppResourceId *string `json:"containerAppResourceId,omitempty"`
	State                  *string `json:"state,omitempty"`
}

type sharePointConnectorStagedApi struct {
	Item *sharePointConnectorStagedJson `json:"item"`
}

func NewSharePointConnectorStaged() resource.Resource {
	return &SharePointConnectorStaged{}
}

func (*SharePointConnectorStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_sharepoint_connector_staged"
}

func (*SharePointConnectorStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged SharePoint connector installation for a Microsoft Entra directory. Staging names the Container App to deploy the connector as. The connector has no P0 service account of its own: P0 calls it as the Entra Agent Identity Creator's service account for the same directory, which the connector's app registration must trust.

Use the read-only ` + "`service_account_id`" + ` attribute of ` + "`p0_entra_agent_identity_creator_staged`" + ` as the subject of that app registration's federated identity credential, and deploy the connector's Container App with the name and resource ID this resource returns, in the Entra Agent Identity Creator's Container Apps environment. Once the connector is deployed, create a ` + "`p0_sharepoint_connector`" + ` resource with the same ` + "`directory_id`" + ` to complete the installation.

**Prerequisite:** the Entra Agent Identity Creator (` + "`p0_entra_agent_identity_creator`" + `) must be installed for the same directory. P0 refuses to stage the connector otherwise.

` + previewNote + `

` + docsLink,
		Attributes: map[string]schema.Attribute{
			"directory_id": directoryIdAttribute(`The Microsoft Entra directory (tenant) ID, in lowercase, whose SharePoint sites P0 grants access to. The Entra Agent Identity Creator must be installed for it.`),
			"connector_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name to give the connector's Container App and its managed identity`,
			},
			"container_app_resource_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The resource ID P0 reads the connector's Container App at: in the Entra Agent Identity Creator's subscription and resource group, named ` + "`connector_name`",
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *SharePointConnectorStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *SharePointConnectorStaged) getId(data any) *string {
	model, ok := data.(*sharePointConnectorStagedModel)
	if !ok {
		return nil
	}
	str := model.DirectoryId.ValueString()
	return &str
}

func (r *SharePointConnectorStaged) getItemJson(json any) any {
	inner, ok := json.(*sharePointConnectorStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SharePointConnectorStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*sharePointConnectorStagedJson)
	if !ok {
		return nil
	}

	return &sharePointConnectorStagedModel{
		DirectoryId:            types.StringValue(id),
		ConnectorName:          types.StringPointerValue(jsonv.ConnectorName),
		ContainerAppResourceId: types.StringPointerValue(jsonv.ContainerAppResourceId),
		State:                  types.StringPointerValue(jsonv.State),
	}
}

func (r *SharePointConnectorStaged) toJson(data any) any {
	if _, ok := data.(*sharePointConnectorStagedModel); !ok {
		return nil
	}
	// The directory is the item ID, and P0 assigns every other field, so the body is
	// empty.
	return &sharePointConnectorStagedJson{}
}

func (r *SharePointConnectorStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json sharePointConnectorStagedApi
	var data sharePointConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *SharePointConnectorStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &sharePointConnectorStagedApi{}, &sharePointConnectorStagedModel{})
}

// Every attribute is either the item ID, which replaces the resource, or computed, so
// Terraform never plans an update.
func (r *SharePointConnectorStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json sharePointConnectorStagedApi
	var data sharePointConnectorStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *SharePointConnectorStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &sharePointConnectorStagedModel{})
}

func (r *SharePointConnectorStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("directory_id"), req, resp)
}
