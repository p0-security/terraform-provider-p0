package installsnowflakeai

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

var _ resource.Resource = &SnowflakeAiCredential{}
var _ resource.ResourceWithConfigure = &SnowflakeAiCredential{}
var _ resource.ResourceWithImportState = &SnowflakeAiCredential{}
var _ resource.ResourceWithValidateConfig = &SnowflakeAiCredential{}

type SnowflakeAiCredential struct {
	installer *common.Install
}

type snowflakeAiCredentialModel struct {
	Id            types.String                                        `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	State         types.String                                        `tfsdk:"state"`
}

type snowflakeAiCredentialJson struct {
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type snowflakeAiCredentialApi struct {
	Item *snowflakeAiCredentialJson `json:"item"`
}

func NewSnowflakeAiCredential() resource.Resource {
	return &SnowflakeAiCredential{}
}

func (*SnowflakeAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_snowflake_ai"
}

func (*SnowflakeAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Snowflake installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a Snowflake account, with a Snowflake user and role that hold only the requested roles or object privileges.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_snowflake_ai_staged`" + `, deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud), set up P0's connector user and MCP server in the Snowflake account, and add the connector user's private key as a version of the connector's private key secret. Creating this resource verifies that the connector can sign in to the account with that key.

` + betaNote,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute("The `id` of the `p0_snowflake_ai_staged` resource being finalized"),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Snowflake connector runs and stores ` + secretLabel,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_snowflake_ai_staged` resource."),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*SnowflakeAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *SnowflakeAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  SnowflakeAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *SnowflakeAiCredential) getId(data any) *string {
	model, ok := data.(*snowflakeAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *SnowflakeAiCredential) getItemJson(json any) any {
	inner, ok := json.(*snowflakeAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SnowflakeAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*snowflakeAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &snowflakeAiCredentialModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *SnowflakeAiCredential) toJson(data any) any {
	datav, ok := data.(*snowflakeAiCredentialModel)
	if !ok {
		return nil
	}

	json := snowflakeAiCredentialJson{}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *SnowflakeAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json snowflakeAiCredentialApi
	var data snowflakeAiCredentialModel

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

func (r *SnowflakeAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &snowflakeAiCredentialApi{}, &snowflakeAiCredentialModel{})
}

func (r *SnowflakeAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &snowflakeAiCredentialApi{}, &snowflakeAiCredentialModel{})
}

func (r *SnowflakeAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &snowflakeAiCredentialModel{})
}

func (r *SnowflakeAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
