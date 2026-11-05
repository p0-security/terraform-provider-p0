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

var _ resource.Resource = &CloudflareAiCredential{}
var _ resource.ResourceWithConfigure = &CloudflareAiCredential{}
var _ resource.ResourceWithImportState = &CloudflareAiCredential{}
var _ resource.ResourceWithValidateConfig = &CloudflareAiCredential{}

type CloudflareAiCredential struct {
	installer *common.Install
}

type cloudflareAiCredentialModel struct {
	Id            types.String                                        `tfsdk:"id"`
	AccountId     types.String                                        `tfsdk:"account_id"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	OtherGroups   types.String                                        `tfsdk:"other_groups"`
	State         types.String                                        `tfsdk:"state"`
}

type cloudflareAiCredentialJson struct {
	AccountId     *string                                            `json:"accountId,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	OtherGroups   *string                                            `json:"otherGroups,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type cloudflareAiCredentialApi struct {
	Item *cloudflareAiCredentialJson `json:"item"`
}

func NewCloudflareAiCredential() resource.Resource {
	return &CloudflareAiCredential{}
}

func (*CloudflareAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_cloudflare_ai"
}

func (*CloudflareAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Cloudflare installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a Cloudflare account, with an account API token that holds only the requested permission groups.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_cloudflare_ai_staged`" + `, deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud), and add the install token as a version of the connector's install token secret. The install token is an account API token with the Account API Tokens Edit and Zone Read permission groups. Creating this resource verifies that the connector can read the install token and use it on the account.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute("The `id` of the `p0_cloudflare_ai_staged` resource being finalized"),
			"account_id": accountIdAttribute(" Must match the `p0_cloudflare_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Cloudflare connector runs and stores the Cloudflare account's install token`,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_cloudflare_ai_staged` resource."),
			},
			// No UseStateForUnknown: P0 reads the groups again on each verify, so the value can change.
			"other_groups": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The install token's permission groups beyond Account API Tokens Edit and Zone Read, joined by ", ". P0 needs only those two; remove the others from the install token.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (*CloudflareAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *CloudflareAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CloudflareAiCredential) getId(data any) *string {
	model, ok := data.(*cloudflareAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *CloudflareAiCredential) getItemJson(json any) any {
	inner, ok := json.(*cloudflareAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *CloudflareAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*cloudflareAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &cloudflareAiCredentialModel{
		Id:            types.StringValue(id),
		AccountId:     types.StringPointerValue(jsonv.AccountId),
		SecretManager: &secretManager,
		OtherGroups:   types.StringPointerValue(jsonv.OtherGroups),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *CloudflareAiCredential) toJson(data any) any {
	datav, ok := data.(*cloudflareAiCredentialModel)
	if !ok {
		return nil
	}

	json := cloudflareAiCredentialJson{
		AccountId: datav.AccountId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *CloudflareAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json cloudflareAiCredentialApi
	var data cloudflareAiCredentialModel

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

func (r *CloudflareAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &cloudflareAiCredentialApi{}, &cloudflareAiCredentialModel{})
}

func (r *CloudflareAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &cloudflareAiCredentialApi{}, &cloudflareAiCredentialModel{})
}

func (r *CloudflareAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &cloudflareAiCredentialModel{})
}

func (r *CloudflareAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
