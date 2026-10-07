package installgithubai

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

var _ resource.Resource = &GithubAiCredential{}
var _ resource.ResourceWithConfigure = &GithubAiCredential{}
var _ resource.ResourceWithImportState = &GithubAiCredential{}

type GithubAiCredential struct {
	installer *common.Install
}

type githubAiCredentialModel struct {
	Id            types.String                                 `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	AppId         types.String                                 `tfsdk:"app_id"`
	State         types.String                                 `tfsdk:"state"`
}

type githubAiCredentialJson struct {
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	AppId         *string                                     `json:"appId,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type githubAiCredentialApi struct {
	Item *githubAiCredentialJson `json:"item"`
}

func NewGithubAiCredential() resource.Resource {
	return &GithubAiCredential{}
}

func (*GithubAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_github_ai"
}

func (*GithubAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GitHub App installation.

Installing the GitHub App allows P0 to grant machines just-in-time, scoped access to your GitHub organization's repositories.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_github_ai_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is deployed. After you create it, add the GitHub App's private key as a version of the connector's private key secret; the connector cannot mint access tokens until that version exists.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": common.FixedAttribute("The `id` of the `p0_github_ai_staged` resource being finalized"),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitHub connector runs and stores the GitHub App's private key`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_github_ai_staged` resource."),
			},
			"app_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the GitHub App that P0 uses to grant access`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GithubAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GithubAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GithubAiCredential) getId(data any) *string {
	model, ok := data.(*githubAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GithubAiCredential) getItemJson(json any) any {
	inner, ok := json.(*githubAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GithubAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*githubAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &githubAiCredentialModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		AppId:         types.StringPointerValue(jsonv.AppId),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GithubAiCredential) toJson(data any) any {
	datav, ok := data.(*githubAiCredentialModel)
	if !ok {
		return nil
	}

	json := githubAiCredentialJson{
		AppId: datav.AppId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *GithubAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json githubAiCredentialApi
	var data githubAiCredentialModel

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

func (r *GithubAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &githubAiCredentialApi{}, &githubAiCredentialModel{})
}

func (r *GithubAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &githubAiCredentialApi{}, &githubAiCredentialModel{})
}

func (r *GithubAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &githubAiCredentialModel{})
}

func (r *GithubAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
