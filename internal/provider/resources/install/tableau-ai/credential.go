package installtableauai

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

var _ resource.Resource = &TableauAiCredential{}
var _ resource.ResourceWithConfigure = &TableauAiCredential{}
var _ resource.ResourceWithImportState = &TableauAiCredential{}
var _ resource.ResourceWithValidateConfig = &TableauAiCredential{}

type TableauAiCredential struct {
	installer *common.Install
}

type tableauAiCredentialModel struct {
	Id            types.String                                        `tfsdk:"id"`
	Server        types.String                                        `tfsdk:"server"`
	ContentUrl    types.String                                        `tfsdk:"content_url"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsModel `tfsdk:"secret_manager"`
	AdminUser     types.String                                        `tfsdk:"admin_user"`
	AgentMailbox  types.String                                        `tfsdk:"agent_mailbox"`
	State         types.String                                        `tfsdk:"state"`
}

type tableauAiCredentialJson struct {
	Server        *string                                            `json:"server,omitempty"`
	ContentUrl    *string                                            `json:"contentUrl,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	AdminUser     *string                                            `json:"adminUser,omitempty"`
	AgentMailbox  *string                                            `json:"agentMailbox,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type tableauAiCredentialApi struct {
	Item *tableauAiCredentialJson `json:"item"`
}

func NewTableauAiCredential() resource.Resource {
	return &TableauAiCredential{}
}

func (*TableauAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_tableau_ai"
}

func (*TableauAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Tableau site installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to the projects of a Tableau Cloud site, through an agent user on the site for each machine.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_tableau_ai_staged`" + ` and deploy the connector (its Lambda function on AWS, or its Cloud Run service on Google Cloud). Creating this resource verifies that the connector is deployed. After you create it, add the admin connected app's client ID, secret ID and secret value as a JSON version of the connector's admin connected app secret; the connector cannot grant access until that version exists.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute("The `id` of the `p0_tableau_ai_staged` resource being finalized"),
			"server":      serverAttribute(" Must match the `p0_tableau_ai_staged` resource."),
			"content_url": contentUrlAttribute(" Must match the `p0_tableau_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Tableau connector runs and stores the admin connected app's secret`,
				Attributes:          installvaultedcredential.FinalSecretManagerWithAwsAttributes(secretLabel, " Must match the `p0_tableau_ai_staged` resource."),
			},
			"admin_user": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The email of the site administrator that P0's admin connected app signs in as`,
			},
			"agent_mailbox": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The email address that P0 plus-addresses to name agent users. Tableau sends each new agent user an invitation, so this mailbox gets one email per agent user.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (*TableauAiCredential) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *TableauAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  TableauAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *TableauAiCredential) getId(data any) *string {
	model, ok := data.(*tableauAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *TableauAiCredential) getItemJson(json any) any {
	inner, ok := json.(*tableauAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *TableauAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*tableauAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsFromJson(jsonv.SecretManager)
	return &tableauAiCredentialModel{
		Id:            types.StringValue(id),
		Server:        types.StringPointerValue(jsonv.Server),
		ContentUrl:    types.StringPointerValue(jsonv.ContentUrl),
		SecretManager: &secretManager,
		AdminUser:     types.StringPointerValue(jsonv.AdminUser),
		AgentMailbox:  types.StringPointerValue(jsonv.AgentMailbox),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *TableauAiCredential) toJson(data any) any {
	datav, ok := data.(*tableauAiCredentialModel)
	if !ok {
		return nil
	}

	json := tableauAiCredentialJson{
		Server:       datav.Server.ValueStringPointer(),
		ContentUrl:   datav.ContentUrl.ValueStringPointer(),
		AdminUser:    datav.AdminUser.ValueStringPointer(),
		AgentMailbox: datav.AgentMailbox.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *TableauAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json tableauAiCredentialApi
	var data tableauAiCredentialModel

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

func (r *TableauAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &tableauAiCredentialApi{}, &tableauAiCredentialModel{})
}

func (r *TableauAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &tableauAiCredentialApi{}, &tableauAiCredentialModel{})
}

func (r *TableauAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &tableauAiCredentialModel{})
}

func (r *TableauAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
