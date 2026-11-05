package installcloudflareai

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

var _ resource.Resource = &CloudflareAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &CloudflareAiCredentialStaged{}
var _ resource.ResourceWithImportState = &CloudflareAiCredentialStaged{}
var _ resource.ResourceWithValidateConfig = &CloudflareAiCredentialStaged{}

type CloudflareAiCredentialStaged struct {
	installer *common.Install
}

type cloudflareAiCredentialStagedModel struct {
	Id            types.String                                              `tfsdk:"id"`
	AccountId     types.String                                              `tfsdk:"account_id"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsStagedModel `tfsdk:"secret_manager"`
	State         types.String                                              `tfsdk:"state"`
}

type cloudflareAiCredentialStagedJson struct {
	AccountId     *string                                            `json:"accountId,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type cloudflareAiCredentialStagedApi struct {
	Item *cloudflareAiCredentialStagedJson `json:"item"`
}

func NewCloudflareAiCredentialStaged() resource.Resource {
	return &CloudflareAiCredentialStaged{}
}

func (*CloudflareAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_cloudflare_ai_staged"
}

func (*CloudflareAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Cloudflare installation for agentic access. Staging generates the identifiers needed to deploy P0's Cloudflare connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector and its install token secret: a Lambda function with ` + "`aws-sm`" + `, or a Cloud Run service with ` + "`gcp-sm`" + `. Once the connector is deployed and the install token is stored in its secret, create a ` + "`p0_cloudflare_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the AWS account with AWS Secrets Manager, or on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute(`An identifier for the Cloudflare account, made of letters, digits and hyphens, up to 64 characters. P0 puts it in the name of every secret and token that it makes.`),
			"account_id": accountIdAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Cloudflare connector runs and stores the Cloudflare account's install token`,
				Attributes:          installvaultedcredential.SecretManagerWithAwsAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*CloudflareAiCredentialStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *CloudflareAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  CloudflareAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *CloudflareAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*cloudflareAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *CloudflareAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*cloudflareAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *CloudflareAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*cloudflareAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsStagedFromJson(jsonv.SecretManager)
	return &cloudflareAiCredentialStagedModel{
		Id:            types.StringValue(id),
		AccountId:     types.StringPointerValue(jsonv.AccountId),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *CloudflareAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*cloudflareAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := cloudflareAiCredentialStagedJson{
		AccountId: datav.AccountId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *CloudflareAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json cloudflareAiCredentialStagedApi
	var data cloudflareAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *CloudflareAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &cloudflareAiCredentialStagedApi{}, &cloudflareAiCredentialStagedModel{})
}

func (r *CloudflareAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json cloudflareAiCredentialStagedApi
	var data cloudflareAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *CloudflareAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &cloudflareAiCredentialStagedModel{})
}

func (r *CloudflareAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
