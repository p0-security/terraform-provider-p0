package installgithubapp

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

var _ resource.Resource = &GithubAppCredential{}
var _ resource.ResourceWithConfigure = &GithubAppCredential{}
var _ resource.ResourceWithImportState = &GithubAppCredential{}

type GithubAppCredential struct {
	installer *common.Install
}

type githubAppCredentialModel struct {
	Id            types.String        `tfsdk:"id"`
	SecretManager *secretManagerModel `tfsdk:"secret_manager"`
	AppId         types.String        `tfsdk:"app_id"`
	State         types.String        `tfsdk:"state"`
}

type githubAppCredentialJson struct {
	SecretManager *secretManagerJson `json:"secretManager,omitempty"`
	AppId         *string            `json:"appId,omitempty"`
	State         *string            `json:"state,omitempty"`
}

type githubAppCredentialApi struct {
	Item *githubAppCredentialJson `json:"item"`
}

func NewGithubAppCredential() resource.Resource {
	return &GithubAppCredential{}
}

func (*GithubAppCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_github_app"
}

func (*GithubAppCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	secretManager := secretManagerAttributes(" Must match the `p0_github_app_staged` resource.")
	secretManager["connector_service_uri"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: `The invocation URL of the connector's Cloud Run service, resolved by P0 during install`,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: `A GitHub App installation.

Installing the GitHub App allows P0 to grant machines just-in-time, scoped access to your GitHub organization's repositories.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_github_app_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is deployed. After you create it, add the GitHub App's private key as a version of the connector's private key secret; the connector cannot mint access tokens until that version exists.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute("The `id` of the `p0_github_app_staged` resource being finalized"),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitHub connector runs and stores the GitHub App's private key`,
				Attributes:          secretManager,
			},
			"app_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the GitHub App that P0 uses to grant access`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GithubAppCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GithubAppKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GithubAppCredential) getId(data any) *string {
	model, ok := data.(*githubAppCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GithubAppCredential) getItemJson(json any) any {
	inner, ok := json.(*githubAppCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GithubAppCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*githubAppCredentialJson)
	if !ok {
		return nil
	}
	if !requireSecretManager(diags, id, jsonv.SecretManager) {
		return nil
	}

	return &githubAppCredentialModel{
		Id: types.StringValue(id),
		SecretManager: &secretManagerModel{
			secretManagerStagedModel: secretManagerStagedFromJson(jsonv.SecretManager),
			ConnectorServiceUri:      types.StringPointerValue(jsonv.SecretManager.ConnectorServiceUri),
		},
		AppId: types.StringPointerValue(jsonv.AppId),
		State: types.StringPointerValue(jsonv.State),
	}
}

func (r *GithubAppCredential) toJson(data any) any {
	datav, ok := data.(*githubAppCredentialModel)
	if !ok {
		return nil
	}

	json := githubAppCredentialJson{
		AppId: datav.AppId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.toJson()
	}
	return &json
}

func (r *GithubAppCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json githubAppCredentialApi
	var data githubAppCredentialModel

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

func (r *GithubAppCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &githubAppCredentialApi{}, &githubAppCredentialModel{})
}

func (r *GithubAppCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &githubAppCredentialApi{}, &githubAppCredentialModel{})
}

func (r *GithubAppCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &githubAppCredentialModel{})
}

func (r *GithubAppCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
