package installgitguardianai

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

var _ resource.Resource = &GitguardianAiCredential{}
var _ resource.ResourceWithConfigure = &GitguardianAiCredential{}
var _ resource.ResourceWithImportState = &GitguardianAiCredential{}
var _ resource.ResourceWithValidateConfig = &GitguardianAiCredential{}

type GitguardianAiCredential struct {
	installer *common.Install
}

type gitguardianAiCredentialModel struct {
	Id            types.String                                        `tfsdk:"id"`
	Region        types.String                                        `tfsdk:"region"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	State         types.String                                        `tfsdk:"state"`
}

type gitguardianAiCredentialJson struct {
	Region        *regionJson                                        `json:"region,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type gitguardianAiCredentialApi struct {
	Item *gitguardianAiCredentialJson `json:"item"`
}

func NewGitguardianAiCredential() resource.Resource {
	return &GitguardianAiCredential{}
}

func (*GitguardianAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gitguardian_ai"
}

func (*GitguardianAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GitGuardian installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a GitGuardian workspace, with an API token that holds only the requested scopes.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_gitguardian_ai_staged`" + `, deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud), and add the workspace's admin token as a version of the connector's admin token secret. Creating this resource has the connector check the admin token: GitGuardian accepts it in the region, it is active, it belongs to this workspace, and it holds ` + "`api_tokens:read`" + ` and ` + "`api_tokens:write`" + `.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":     idAttribute("The `id` of the `p0_gitguardian_ai_staged` resource being finalized"),
			"region": regionAttribute(" Must match the `p0_gitguardian_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitGuardian connector runs and stores the workspace's admin token`,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_gitguardian_ai_staged` resource."),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*GitguardianAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *GitguardianAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GitguardianAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GitguardianAiCredential) getId(data any) *string {
	model, ok := data.(*gitguardianAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GitguardianAiCredential) getItemJson(json any) any {
	inner, ok := json.(*gitguardianAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GitguardianAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*gitguardianAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &gitguardianAiCredentialModel{
		Id:            types.StringValue(id),
		Region:        regionFromJson(jsonv.Region),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GitguardianAiCredential) toJson(data any) any {
	datav, ok := data.(*gitguardianAiCredentialModel)
	if !ok {
		return nil
	}

	json := gitguardianAiCredentialJson{
		Region: regionToJson(datav.Region),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *GitguardianAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gitguardianAiCredentialApi
	var data gitguardianAiCredentialModel

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

func (r *GitguardianAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gitguardianAiCredentialApi{}, &gitguardianAiCredentialModel{})
}

func (r *GitguardianAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gitguardianAiCredentialApi{}, &gitguardianAiCredentialModel{})
}

func (r *GitguardianAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gitguardianAiCredentialModel{})
}

func (r *GitguardianAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
