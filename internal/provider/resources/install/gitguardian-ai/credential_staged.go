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

var _ resource.Resource = &GitguardianAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &GitguardianAiCredentialStaged{}
var _ resource.ResourceWithImportState = &GitguardianAiCredentialStaged{}
var _ resource.ResourceWithValidateConfig = &GitguardianAiCredentialStaged{}

type GitguardianAiCredentialStaged struct {
	installer *common.Install
}

type gitguardianAiCredentialStagedModel struct {
	Id            types.String                                              `tfsdk:"id"`
	Region        types.String                                              `tfsdk:"region"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsStagedModel `tfsdk:"secret_manager"`
	State         types.String                                              `tfsdk:"state"`
}

type gitguardianAiCredentialStagedJson struct {
	Region        *regionJson                                        `json:"region,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type gitguardianAiCredentialStagedApi struct {
	Item *gitguardianAiCredentialStagedJson `json:"item"`
}

func NewGitguardianAiCredentialStaged() resource.Resource {
	return &GitguardianAiCredentialStaged{}
}

func (*GitguardianAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gitguardian_ai_staged"
}

func (*GitguardianAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GitGuardian installation for agentic access. Staging generates the identifiers needed to deploy P0's GitGuardian connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector and its admin token secret: a Lambda function with ` + "`aws-sm`" + `, or a Cloud Run service with ` + "`gcp-sm`" + `. Once the connector is deployed, create a ` + "`p0_gitguardian_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the AWS account with AWS Secrets Manager, or on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":     idAttribute(`The GitGuardian workspace ID, as digits. P0 puts it in the name of every secret that it makes.`),
			"region": regionAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's GitGuardian connector runs and stores the workspace's admin token`,
				Attributes:          installvaultedcredential.SecretManagerWithAwsAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*GitguardianAiCredentialStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *GitguardianAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GitguardianAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*gitguardianAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GitguardianAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*gitguardianAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GitguardianAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*gitguardianAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsStagedFromJson(jsonv.SecretManager)
	return &gitguardianAiCredentialStagedModel{
		Id:            types.StringValue(id),
		Region:        regionFromJson(jsonv.Region),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *GitguardianAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*gitguardianAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := gitguardianAiCredentialStagedJson{
		Region: regionToJson(datav.Region),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *GitguardianAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gitguardianAiCredentialStagedApi
	var data gitguardianAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GitguardianAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gitguardianAiCredentialStagedApi{}, &gitguardianAiCredentialStagedModel{})
}

func (r *GitguardianAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gitguardianAiCredentialStagedApi
	var data gitguardianAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GitguardianAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gitguardianAiCredentialStagedModel{})
}

func (r *GitguardianAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
