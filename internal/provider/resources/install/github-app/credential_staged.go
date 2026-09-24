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

var _ resource.Resource = &GithubAppCredentialStaged{}
var _ resource.ResourceWithConfigure = &GithubAppCredentialStaged{}
var _ resource.ResourceWithImportState = &GithubAppCredentialStaged{}

type GithubAppCredentialStaged struct {
	installer *common.Install
}

type githubAppCredentialStagedModel struct {
	Id            types.String              `tfsdk:"id"`
	SecretManager *secretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String              `tfsdk:"state"`
}

type githubAppCredentialStagedJson struct {
	SecretManager *secretManagerJson `json:"secretManager,omitempty"`
	State         *string            `json:"state,omitempty"`
}

type githubAppCredentialStagedApi struct {
	Item *githubAppCredentialStagedJson `json:"item"`
}

func NewGithubAppCredentialStaged() resource.Resource {
	return &GithubAppCredentialStaged{}
}

func (*GithubAppCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_github_app_staged"
}

func (*GithubAppCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GitHub App installation. Staging generates the identifiers needed to deploy P0's GitHub connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its private key secret. Once the connector is deployed, create a ` + "`p0_github_app`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute(`The login of the GitHub organization that the GitHub App is installed on`),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitHub connector runs and stores the GitHub App's private key`,
				Attributes:          secretManagerAttributes(""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GithubAppCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GithubAppCredentialStaged) getId(data any) *string {
	model, ok := data.(*githubAppCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GithubAppCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*githubAppCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GithubAppCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*githubAppCredentialStagedJson)
	if !ok {
		return nil
	}
	if !requireSecretManager(diags, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := secretManagerStagedFromJson(jsonv.SecretManager)
	return &githubAppCredentialStagedModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GithubAppCredentialStaged) toJson(data any) any {
	datav, ok := data.(*githubAppCredentialStagedModel)
	if !ok {
		return nil
	}

	json := githubAppCredentialStagedJson{}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.toJson()
	}
	return &json
}

func (r *GithubAppCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json githubAppCredentialStagedApi
	var data githubAppCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GithubAppCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &githubAppCredentialStagedApi{}, &githubAppCredentialStagedModel{})
}

func (r *GithubAppCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json githubAppCredentialStagedApi
	var data githubAppCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GithubAppCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &githubAppCredentialStagedModel{})
}

func (r *GithubAppCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
