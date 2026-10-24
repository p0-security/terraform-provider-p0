package installgitlabai

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

var _ resource.Resource = &GitlabAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &GitlabAiCredentialStaged{}
var _ resource.ResourceWithImportState = &GitlabAiCredentialStaged{}

type GitlabAiCredentialStaged struct {
	installer *common.Install
}

type gitlabAiCredentialStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type gitlabAiCredentialStagedJson struct {
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type gitlabAiCredentialStagedApi struct {
	Item *gitlabAiCredentialStagedJson `json:"item"`
}

func NewGitlabAiCredentialStaged() resource.Resource {
	return &GitlabAiCredentialStaged{}
}

func (*GitlabAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gitlab_ai_staged"
}

func (*GitlabAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GitLab installation for agentic access. Staging generates the identifiers needed to deploy P0's GitLab connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its admin token secret. Once the connector is deployed, create a ` + "`p0_gitlab_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute(`An identifier for the GitLab top-level group, made of letters, digits and hyphens. P0 puts it in the name of every secret that it makes.`),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitLab connector runs and stores the GitLab group's admin token`,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GitlabAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GitlabAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GitlabAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*gitlabAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GitlabAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*gitlabAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GitlabAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*gitlabAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &gitlabAiCredentialStagedModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GitlabAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*gitlabAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := gitlabAiCredentialStagedJson{}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *GitlabAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gitlabAiCredentialStagedApi
	var data gitlabAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GitlabAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gitlabAiCredentialStagedApi{}, &gitlabAiCredentialStagedModel{})
}

func (r *GitlabAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gitlabAiCredentialStagedApi
	var data gitlabAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GitlabAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gitlabAiCredentialStagedModel{})
}

func (r *GitlabAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
