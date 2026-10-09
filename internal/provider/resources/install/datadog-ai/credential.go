package installdatadogai

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

var _ resource.Resource = &DatadogAiCredential{}
var _ resource.ResourceWithConfigure = &DatadogAiCredential{}
var _ resource.ResourceWithImportState = &DatadogAiCredential{}

type DatadogAiCredential struct {
	installer *common.Install
}

type datadogAiCredentialModel struct {
	Id               types.String                                 `tfsdk:"id"`
	Site             types.String                                 `tfsdk:"site"`
	SecretManager    *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ServiceAccountId types.String                                 `tfsdk:"service_account_id"`
	State            types.String                                 `tfsdk:"state"`
}

type datadogAiCredentialJson struct {
	Site             *siteJson                                   `json:"site,omitempty"`
	SecretManager    *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ServiceAccountId *string                                     `json:"serviceAccountId,omitempty"`
	State            *string                                     `json:"state,omitempty"`
}

type datadogAiCredentialApi struct {
	Item *datadogAiCredentialJson `json:"item"`
}

func NewDatadogAiCredential() resource.Resource {
	return &DatadogAiCredential{}
}

func (*DatadogAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_datadog_ai"
}

func (*DatadogAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Datadog installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a Datadog organization, with a service access token that holds only the requested scopes.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_datadog_ai_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is deployed. After you create it, add the organization's API key and application key as a version of the connector's admin keys secret; the connector cannot create access tokens until that version exists.

` + common.NotePreview,
		Attributes: map[string]schema.Attribute{
			"id":   idAttribute("The `id` of the `p0_datadog_ai_staged` resource being finalized"),
			"site": siteAttribute(" Must match the `p0_datadog_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Datadog connector runs and stores the Datadog organization's admin keys`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_datadog_ai_staged` resource."),
			},
			"service_account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the Datadog service account that owns the access tokens P0 creates. Its role limits which scopes can be requested.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *DatadogAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  DatadogAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *DatadogAiCredential) getId(data any) *string {
	model, ok := data.(*datadogAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DatadogAiCredential) getItemJson(json any) any {
	inner, ok := json.(*datadogAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *DatadogAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*datadogAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &datadogAiCredentialModel{
		Id:               types.StringValue(id),
		Site:             siteFromJson(jsonv.Site),
		SecretManager:    &secretManager,
		ServiceAccountId: types.StringPointerValue(jsonv.ServiceAccountId),
		State:            types.StringPointerValue(jsonv.State),
	}
}

func (r *DatadogAiCredential) toJson(data any) any {
	datav, ok := data.(*datadogAiCredentialModel)
	if !ok {
		return nil
	}

	json := datadogAiCredentialJson{
		Site:             siteToJson(datav.Site),
		ServiceAccountId: datav.ServiceAccountId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *DatadogAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json datadogAiCredentialApi
	var data datadogAiCredentialModel

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

func (r *DatadogAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &datadogAiCredentialApi{}, &datadogAiCredentialModel{})
}

func (r *DatadogAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &datadogAiCredentialApi{}, &datadogAiCredentialModel{})
}

func (r *DatadogAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &datadogAiCredentialModel{})
}

func (r *DatadogAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
