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

var _ resource.Resource = &GitlabAiCredential{}
var _ resource.ResourceWithConfigure = &GitlabAiCredential{}
var _ resource.ResourceWithImportState = &GitlabAiCredential{}
var _ resource.ResourceWithValidateConfig = &GitlabAiCredential{}

type GitlabAiCredential struct {
	installer *common.Install
}

type gitlabAiCredentialModel struct {
	Id            types.String                                        `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	GroupId       types.String                                        `tfsdk:"group_id"`
	State         types.String                                        `tfsdk:"state"`
}

type gitlabAiCredentialJson struct {
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	GroupId       *string                                            `json:"groupId,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type gitlabAiCredentialApi struct {
	Item *gitlabAiCredentialJson `json:"item"`
}

func NewGitlabAiCredential() resource.Resource {
	return &GitlabAiCredential{}
}

func (*GitlabAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gitlab_ai"
}

func (*GitlabAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GitLab installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to projects in a GitLab.com top-level group. Each grant gets its own GitLab service account, a member of the requested projects with the requested role, and a token for the GitLab MCP server.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_gitlab_ai_staged`" + ` and deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud). Creating this resource verifies that the connector is deployed. After you create it, add the admin service account's token as a version of the connector's admin token secret; the connector cannot grant access until that version exists.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute("The `id` of the `p0_gitlab_ai_staged` resource being finalized"),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitLab connector runs and stores the GitLab group's admin token`,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_gitlab_ai_staged` resource."),
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the GitLab.com top-level group, shown on the group's overview page. P0 creates one service account in it for each grant.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (*GitlabAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *GitlabAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GitlabAiCredential) getId(data any) *string {
	model, ok := data.(*gitlabAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GitlabAiCredential) getItemJson(json any) any {
	inner, ok := json.(*gitlabAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GitlabAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*gitlabAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &gitlabAiCredentialModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		GroupId:       types.StringPointerValue(jsonv.GroupId),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GitlabAiCredential) toJson(data any) any {
	datav, ok := data.(*gitlabAiCredentialModel)
	if !ok {
		return nil
	}

	json := gitlabAiCredentialJson{
		GroupId: datav.GroupId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *GitlabAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gitlabAiCredentialApi
	var data gitlabAiCredentialModel

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

func (r *GitlabAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gitlabAiCredentialApi{}, &gitlabAiCredentialModel{})
}

func (r *GitlabAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gitlabAiCredentialApi{}, &gitlabAiCredentialModel{})
}

func (r *GitlabAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gitlabAiCredentialModel{})
}

func (r *GitlabAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
