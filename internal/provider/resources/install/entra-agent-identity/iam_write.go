package installentraagentidentity

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

var _ resource.Resource = &EntraAgentIdentityIamWrite{}
var _ resource.ResourceWithConfigure = &EntraAgentIdentityIamWrite{}
var _ resource.ResourceWithImportState = &EntraAgentIdentityIamWrite{}

type EntraAgentIdentityIamWrite struct {
	installer *common.Install
}

type entraAgentIdentityIamWriteModel struct {
	DirectoryId            types.String `tfsdk:"directory_id"`
	ConnectorClientId      types.String `tfsdk:"connector_client_id"`
	ServiceAccountEmail    types.String `tfsdk:"service_account_email"`
	ServiceAccountId       types.String `tfsdk:"service_account_id"`
	ContainerAppResourceId types.String `tfsdk:"container_app_resource_id"`
	EnvironmentId          types.String `tfsdk:"environment_id"`
	ConnectorServiceUri    types.String `tfsdk:"connector_service_uri"`
	ForgeClientId          types.String `tfsdk:"forge_client_id"`
	State                  types.String `tfsdk:"state"`
}

type entraAgentIdentityIamWriteJson struct {
	ConnectorClientId      *string `json:"connectorClientId,omitempty"`
	ServiceAccountEmail    *string `json:"serviceAccountEmail,omitempty"`
	ServiceAccountId       *string `json:"serviceAccountId,omitempty"`
	ContainerAppResourceId *string `json:"containerAppResourceId,omitempty"`
	EnvironmentId          *string `json:"environmentId,omitempty"`
	ConnectorServiceUri    *string `json:"connectorServiceUri,omitempty"`
	ForgeClientId          *string `json:"forgeClientId,omitempty"`
	State                  *string `json:"state,omitempty"`
}

type entraAgentIdentityIamWriteApi struct {
	Item *entraAgentIdentityIamWriteJson `json:"item"`
}

func NewEntraAgentIdentityIamWrite() resource.Resource {
	return &EntraAgentIdentityIamWrite{}
}

func (*EntraAgentIdentityIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_entra_agent_identity_creator"
}

func (*EntraAgentIdentityIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An Entra Agent Identity Creator installation for a Microsoft Entra directory.

Installing the Entra Agent Identity Creator lets P0 create an app registration in this directory for each agentic grant, so that Microsoft 365 integrations in the directory can grant that app access.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_entra_agent_identity_creator_staged`" + ` and deploy the connector's Container App and app registration. Creating this resource reads the Container App as that app registration and checks that P0 can reach the connector. A new federated identity credential can take a few minutes to start working, so the first apply can take several minutes.

` + previewNote + `

` + docsLink,
		Attributes: map[string]schema.Attribute{
			"directory_id": directoryIdAttribute(`The ` + "`directory_id`" + ` of the ` + "`p0_entra_agent_identity_creator_staged`" + ` resource being finalized`),
			"connector_client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID, in lowercase, of the connector's app registration, which P0 calls the connector as. It is the Terraform output ` + "`p0_entra_agent_identity_connector_client_id`" + `.`,
				Validators:          []validator.String{guidValidator("connector client ID")},
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the P0 service account that calls the connector`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The numeric ID of the P0 service account that calls the connector. The connector of every Microsoft 365 integration in the directory trusts this account.`,
			},
			"container_app_resource_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The resource ID of the connector's Container App`,
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The resource ID of the Container Apps environment that the connectors of Microsoft 365 integrations in this directory are deployed in`,
			},
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The connector's URL, read from its Container App`,
			},
			"forge_client_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The client ID of the connector's managed identity, read from its Container App. That identity owns every per-grant app registration, and the connectors of Microsoft 365 integrations check ownership against it.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *EntraAgentIdentityIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:           EntraAgentIdentityKey,
		Component:             installresources.IamWrite,
		ProviderData:          data,
		GetId:                 r.getId,
		GetItemJson:           r.getItemJson,
		FromJson:              r.fromJson,
		ToJson:                r.toJson,
		DescribeWriteNotFound: describeNotOffered,
	}
}

// The item is keyed by the directory.
func (r *EntraAgentIdentityIamWrite) getId(data any) *string {
	model, ok := data.(*entraAgentIdentityIamWriteModel)
	if !ok {
		return nil
	}
	str := model.DirectoryId.ValueString()
	return &str
}

func (r *EntraAgentIdentityIamWrite) getItemJson(json any) any {
	inner, ok := json.(*entraAgentIdentityIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *EntraAgentIdentityIamWrite) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*entraAgentIdentityIamWriteJson)
	if !ok {
		return nil
	}

	return &entraAgentIdentityIamWriteModel{
		DirectoryId:            types.StringValue(id),
		ConnectorClientId:      types.StringPointerValue(jsonv.ConnectorClientId),
		ServiceAccountEmail:    types.StringPointerValue(jsonv.ServiceAccountEmail),
		ServiceAccountId:       types.StringPointerValue(jsonv.ServiceAccountId),
		ContainerAppResourceId: types.StringPointerValue(jsonv.ContainerAppResourceId),
		EnvironmentId:          types.StringPointerValue(jsonv.EnvironmentId),
		ConnectorServiceUri:    types.StringPointerValue(jsonv.ConnectorServiceUri),
		ForgeClientId:          types.StringPointerValue(jsonv.ForgeClientId),
		State:                  types.StringPointerValue(jsonv.State),
	}
}

// Sends only the connector's client ID. P0 keeps the staged inputs it already holds,
// and refuses a change to them after staging, so they are never resent. It generates
// every other field.
func (r *EntraAgentIdentityIamWrite) toJson(data any) any {
	datav, ok := data.(*entraAgentIdentityIamWriteModel)
	if !ok {
		return nil
	}
	return &entraAgentIdentityIamWriteJson{
		ConnectorClientId: datav.ConnectorClientId.ValueStringPointer(),
	}
}

func (r *EntraAgentIdentityIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraAgentIdentityIamWriteApi
	var data entraAgentIdentityIamWriteModel

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

func (r *EntraAgentIdentityIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraAgentIdentityIamWriteApi{}, &entraAgentIdentityIamWriteModel{})
}

// P0 runs every install check on configure, so an update posts to configure alone: a
// refused client ID then leaves the installed item, and this resource's state, as they
// were.
func (r *EntraAgentIdentityIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraAgentIdentityIamWriteApi{}, &entraAgentIdentityIamWriteModel{})
}

// Returns the item to the "stage" state rather than deleting it, so that the staged
// resource still owns it.
func (r *EntraAgentIdentityIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &entraAgentIdentityIamWriteModel{})
}

func (r *EntraAgentIdentityIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("directory_id"), req, resp)
}
