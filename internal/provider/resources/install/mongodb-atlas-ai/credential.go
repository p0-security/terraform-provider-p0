package installmongodbatlasai

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
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &MongodbAtlasAiCredential{}
var _ resource.ResourceWithConfigure = &MongodbAtlasAiCredential{}
var _ resource.ResourceWithImportState = &MongodbAtlasAiCredential{}
var _ resource.ResourceWithValidateConfig = &MongodbAtlasAiCredential{}

type MongodbAtlasAiCredential struct {
	installer *common.Install
}

type mongodbAtlasAiCredentialModel struct {
	Id             types.String                                        `tfsdk:"id"`
	AtlasProjectId types.String                                        `tfsdk:"atlas_project_id"`
	SecretManager  *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	ClientId       types.String                                        `tfsdk:"client_id"`
	State          types.String                                        `tfsdk:"state"`
}

type mongodbAtlasAiCredentialJson struct {
	AtlasProjectId *string                                            `json:"atlasProjectId,omitempty"`
	SecretManager  *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	ClientId       *string                                            `json:"clientId,omitempty"`
	State          *string                                            `json:"state,omitempty"`
}

type mongodbAtlasAiCredentialApi struct {
	Item *mongodbAtlasAiCredentialJson `json:"item"`
}

func NewMongodbAtlasAiCredential() resource.Resource {
	return &MongodbAtlasAiCredential{}
}

func (*MongodbAtlasAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_mongodb_atlas_ai"
}

func (*MongodbAtlasAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A MongoDB Atlas installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to an Atlas project through the Atlas-hosted MCP server, with a token for an MCP configuration that holds only the requested project roles.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_mongodb_atlas_ai_staged`" + ` and deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud). Creating this resource verifies that the connector is deployed. After you create it, add the Atlas service account's client secret as a version of the connector's client secret secret; the connector cannot manage MCP configurations until that version exists.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute("The `id` of the `p0_mongodb_atlas_ai_staged` resource being finalized"),
			"atlas_project_id": atlasProjectIdAttribute(" Must match the `p0_mongodb_atlas_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's MongoDB Atlas connector runs and stores the Atlas service account's client secret`,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_mongodb_atlas_ai_staged` resource."),
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID of the Atlas service account that P0 manages MCP configurations with. It needs the Project Owner role in the project.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (*MongodbAtlasAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *MongodbAtlasAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  MongodbAtlasAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *MongodbAtlasAiCredential) getId(data any) *string {
	model, ok := data.(*mongodbAtlasAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *MongodbAtlasAiCredential) getItemJson(json any) any {
	inner, ok := json.(*mongodbAtlasAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *MongodbAtlasAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*mongodbAtlasAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &mongodbAtlasAiCredentialModel{
		Id:             types.StringValue(id),
		AtlasProjectId: types.StringPointerValue(jsonv.AtlasProjectId),
		SecretManager:  &secretManager,
		ClientId:       types.StringPointerValue(jsonv.ClientId),
		State:          types.StringPointerValue(jsonv.State),
	}
}

func (r *MongodbAtlasAiCredential) toJson(data any) any {
	datav, ok := data.(*mongodbAtlasAiCredentialModel)
	if !ok {
		return nil
	}

	json := mongodbAtlasAiCredentialJson{
		AtlasProjectId: datav.AtlasProjectId.ValueStringPointer(),
		ClientId:       datav.ClientId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *MongodbAtlasAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json mongodbAtlasAiCredentialApi
	var data mongodbAtlasAiCredentialModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *MongodbAtlasAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &mongodbAtlasAiCredentialApi{}, &mongodbAtlasAiCredentialModel{})
}

func (r *MongodbAtlasAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &mongodbAtlasAiCredentialApi{}, &mongodbAtlasAiCredentialModel{})
}

func (r *MongodbAtlasAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &mongodbAtlasAiCredentialModel{})
}

func (r *MongodbAtlasAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
