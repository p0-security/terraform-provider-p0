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

var _ resource.Resource = &MongodbAtlasAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &MongodbAtlasAiCredentialStaged{}
var _ resource.ResourceWithImportState = &MongodbAtlasAiCredentialStaged{}
var _ resource.ResourceWithValidateConfig = &MongodbAtlasAiCredentialStaged{}

type MongodbAtlasAiCredentialStaged struct {
	installer *common.Install
}

type mongodbAtlasAiCredentialStagedModel struct {
	Id             types.String                                              `tfsdk:"id"`
	AtlasProjectId types.String                                              `tfsdk:"atlas_project_id"`
	SecretManager  *installvaultedcredential.SecretManagerWithAwsStagedModel `tfsdk:"secret_manager"`
	State          types.String                                              `tfsdk:"state"`
}

type mongodbAtlasAiCredentialStagedJson struct {
	AtlasProjectId *string                                            `json:"atlasProjectId,omitempty"`
	SecretManager  *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State          *string                                            `json:"state,omitempty"`
}

type mongodbAtlasAiCredentialStagedApi struct {
	Item *mongodbAtlasAiCredentialStagedJson `json:"item"`
}

func NewMongodbAtlasAiCredentialStaged() resource.Resource {
	return &MongodbAtlasAiCredentialStaged{}
}

func (*MongodbAtlasAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_mongodb_atlas_ai_staged"
}

func (*MongodbAtlasAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged MongoDB Atlas installation for agentic access. Staging generates the identifiers needed to deploy P0's MongoDB Atlas connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector and its client secret's secret: a Lambda function with ` + "`aws-sm`" + `, or a Cloud Run service with ` + "`gcp-sm`" + `. Once the connector is deployed, create a ` + "`p0_mongodb_atlas_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the AWS account with AWS Secrets Manager, or on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(`An identifier for the Atlas project, made of letters, digits and hyphens. P0 puts it in the name of every secret that it makes.`),
			"atlas_project_id": atlasProjectIdAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's MongoDB Atlas connector runs and stores the Atlas service account's client secret`,
				Attributes:          installvaultedcredential.SecretManagerWithAwsAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*MongodbAtlasAiCredentialStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *MongodbAtlasAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MongodbAtlasAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*mongodbAtlasAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *MongodbAtlasAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*mongodbAtlasAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *MongodbAtlasAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*mongodbAtlasAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsStagedFromJson(jsonv.SecretManager)
	return &mongodbAtlasAiCredentialStagedModel{
		Id:             types.StringValue(id),
		AtlasProjectId: types.StringPointerValue(jsonv.AtlasProjectId),
		SecretManager:  &secretManager,
		State:          types.StringPointerValue(jsonv.State),
	}
}

func (r *MongodbAtlasAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*mongodbAtlasAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := mongodbAtlasAiCredentialStagedJson{
		AtlasProjectId: datav.AtlasProjectId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *MongodbAtlasAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json mongodbAtlasAiCredentialStagedApi
	var data mongodbAtlasAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *MongodbAtlasAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &mongodbAtlasAiCredentialStagedApi{}, &mongodbAtlasAiCredentialStagedModel{})
}

func (r *MongodbAtlasAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json mongodbAtlasAiCredentialStagedApi
	var data mongodbAtlasAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *MongodbAtlasAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &mongodbAtlasAiCredentialStagedModel{})
}

func (r *MongodbAtlasAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
