package installdatadogapp

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

var _ resource.Resource = &DatadogAppCredential{}
var _ resource.ResourceWithConfigure = &DatadogAppCredential{}
var _ resource.ResourceWithImportState = &DatadogAppCredential{}

type DatadogAppCredential struct {
	installer *common.Install
}

type datadogAppCredentialModel struct {
	Id               types.String                                 `tfsdk:"id"`
	Site             types.String                                 `tfsdk:"site"`
	SecretManager    *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ServiceAccountId types.String                                 `tfsdk:"service_account_id"`
	State            types.String                                 `tfsdk:"state"`
}

type datadogAppCredentialJson struct {
	Site             *siteJson                                   `json:"site,omitempty"`
	SecretManager    *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ServiceAccountId *string                                     `json:"serviceAccountId,omitempty"`
	State            *string                                     `json:"state,omitempty"`
}

type datadogAppCredentialApi struct {
	Item *datadogAppCredentialJson `json:"item"`
}

func NewDatadogAppCredential() resource.Resource {
	return &DatadogAppCredential{}
}

func (*DatadogAppCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_datadog_app"
}

func (*DatadogAppCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Datadog installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a Datadog organization, with a service access token that holds only the requested scopes.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_datadog_app_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is deployed. After you create it, add the organization's API key and application key as a version of the connector's admin keys secret; the connector cannot create access tokens until that version exists.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id":   idAttribute("The `id` of the `p0_datadog_app_staged` resource being finalized"),
			"site": siteAttribute(" Must match the `p0_datadog_app_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Datadog connector runs and stores the Datadog organization's admin keys`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_datadog_app_staged` resource."),
			},
			"service_account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the Datadog service account that owns the access tokens P0 creates. Its role limits which scopes can be requested.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *DatadogAppCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  DatadogAppKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *DatadogAppCredential) getId(data any) *string {
	model, ok := data.(*datadogAppCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DatadogAppCredential) getItemJson(json any) any {
	inner, ok := json.(*datadogAppCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *DatadogAppCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*datadogAppCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &datadogAppCredentialModel{
		Id:               types.StringValue(id),
		Site:             siteFromJson(jsonv.Site),
		SecretManager:    &secretManager,
		ServiceAccountId: types.StringPointerValue(jsonv.ServiceAccountId),
		State:            types.StringPointerValue(jsonv.State),
	}
}

func (r *DatadogAppCredential) toJson(data any) any {
	datav, ok := data.(*datadogAppCredentialModel)
	if !ok {
		return nil
	}

	json := datadogAppCredentialJson{
		Site:             siteToJson(datav.Site),
		ServiceAccountId: datav.ServiceAccountId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *DatadogAppCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json datadogAppCredentialApi
	var data datadogAppCredentialModel

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

func (r *DatadogAppCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &datadogAppCredentialApi{}, &datadogAppCredentialModel{})
}

func (r *DatadogAppCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &datadogAppCredentialApi{}, &datadogAppCredentialModel{})
}

func (r *DatadogAppCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &datadogAppCredentialModel{})
}

func (r *DatadogAppCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
