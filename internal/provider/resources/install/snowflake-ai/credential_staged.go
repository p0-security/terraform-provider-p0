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

var _ resource.Resource = &SnowflakeAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &SnowflakeAiCredentialStaged{}
var _ resource.ResourceWithImportState = &SnowflakeAiCredentialStaged{}

type SnowflakeAiCredentialStaged struct {
	installer *common.Install
}

type snowflakeAiCredentialStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type snowflakeAiCredentialStagedJson struct {
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type snowflakeAiCredentialStagedApi struct {
	Item *snowflakeAiCredentialStagedJson `json:"item"`
}

func NewSnowflakeAiCredentialStaged() resource.Resource {
	return &SnowflakeAiCredentialStaged{}
}

func (*SnowflakeAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_snowflake_ai_staged"
}

func (*SnowflakeAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Snowflake installation for agentic access. Staging generates the identifiers needed to deploy P0's Snowflake connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its private key secret. Once the connector is deployed, create a ` + "`p0_snowflake_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

` + betaNote,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute(`The Snowflake account identifier, in its ` + "`ORGNAME-ACCOUNTNAME`" + ` form (for example ` + "`MYORG-MYACCOUNT`" + `). P0 puts it in the name of every secret that it makes, with each ` + "`_`" + ` written as ` + "`-`" + `.`),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Snowflake connector runs and stores ` + secretLabel,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *SnowflakeAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SnowflakeAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*snowflakeAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *SnowflakeAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*snowflakeAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SnowflakeAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*snowflakeAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &snowflakeAiCredentialStagedModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *SnowflakeAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*snowflakeAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := snowflakeAiCredentialStagedJson{}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *SnowflakeAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json snowflakeAiCredentialStagedApi
	var data snowflakeAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *SnowflakeAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &snowflakeAiCredentialStagedApi{}, &snowflakeAiCredentialStagedModel{})
}

func (r *SnowflakeAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json snowflakeAiCredentialStagedApi
	var data snowflakeAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *SnowflakeAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &snowflakeAiCredentialStagedModel{})
}

func (r *SnowflakeAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
