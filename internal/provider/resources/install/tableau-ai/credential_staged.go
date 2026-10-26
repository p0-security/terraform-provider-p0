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

var _ resource.Resource = &TableauAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &TableauAiCredentialStaged{}
var _ resource.ResourceWithImportState = &TableauAiCredentialStaged{}
var _ resource.ResourceWithValidateConfig = &TableauAiCredentialStaged{}

type TableauAiCredentialStaged struct {
	installer *common.Install
}

type tableauAiCredentialStagedModel struct {
	Id            types.String                                              `tfsdk:"id"`
	Server        types.String                                              `tfsdk:"server"`
	ContentUrl    types.String                                              `tfsdk:"content_url"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsStagedModel `tfsdk:"secret_manager"`
	State         types.String                                              `tfsdk:"state"`
}

type tableauAiCredentialStagedJson struct {
	Server        *string                                            `json:"server,omitempty"`
	ContentUrl    *string                                            `json:"contentUrl,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type tableauAiCredentialStagedApi struct {
	Item *tableauAiCredentialStagedJson `json:"item"`
}

func NewTableauAiCredentialStaged() resource.Resource {
	return &TableauAiCredentialStaged{}
}

func (*TableauAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_tableau_ai_staged"
}

func (*TableauAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Tableau site installation for agentic access. Staging generates the identifiers needed to deploy P0's Tableau connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector and its admin connected app's secret: a Lambda function with ` + "`aws-sm`" + `, or a Cloud Run service with ` + "`gcp-sm`" + `. Once the connector is deployed, create a ` + "`p0_tableau_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the AWS account with AWS Secrets Manager, or on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute(`An identifier for the Tableau site, made of letters, digits and hyphens. P0 puts it in the name of every secret that it makes.`),
			"server":      serverAttribute(""),
			"content_url": contentUrlAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Tableau connector runs and stores the admin connected app's secret`,
				Attributes:          installvaultedcredential.SecretManagerWithAwsAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*TableauAiCredentialStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *TableauAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TableauAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*tableauAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *TableauAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*tableauAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *TableauAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*tableauAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsStagedFromJson(jsonv.SecretManager)
	return &tableauAiCredentialStagedModel{
		Id:            types.StringValue(id),
		Server:        types.StringPointerValue(jsonv.Server),
		ContentUrl:    types.StringPointerValue(jsonv.ContentUrl),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *TableauAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*tableauAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := tableauAiCredentialStagedJson{
		Server:     datav.Server.ValueStringPointer(),
		ContentUrl: datav.ContentUrl.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *TableauAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json tableauAiCredentialStagedApi
	var data tableauAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *TableauAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &tableauAiCredentialStagedApi{}, &tableauAiCredentialStagedModel{})
}

func (r *TableauAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json tableauAiCredentialStagedApi
	var data tableauAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *TableauAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &tableauAiCredentialStagedModel{})
}

func (r *TableauAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
