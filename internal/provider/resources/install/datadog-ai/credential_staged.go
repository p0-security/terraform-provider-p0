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

var _ resource.Resource = &DatadogAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &DatadogAiCredentialStaged{}
var _ resource.ResourceWithImportState = &DatadogAiCredentialStaged{}

type DatadogAiCredentialStaged struct {
	installer *common.Install
}

type datadogAiCredentialStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	Site          types.String                                       `tfsdk:"site"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type datadogAiCredentialStagedJson struct {
	Site          *siteJson                                   `json:"site,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type datadogAiCredentialStagedApi struct {
	Item *datadogAiCredentialStagedJson `json:"item"`
}

func NewDatadogAiCredentialStaged() resource.Resource {
	return &DatadogAiCredentialStaged{}
}

func (*DatadogAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_datadog_ai_staged"
}

func (*DatadogAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Datadog installation for agentic access. Staging generates the identifiers needed to deploy P0's Datadog connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its admin keys secret. Once the connector is deployed, create a ` + "`p0_datadog_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

` + common.NotePreview,
		Attributes: map[string]schema.Attribute{
			"id":   idAttribute(`An identifier for the Datadog organization, made of letters, digits and hyphens. P0 puts it in the name of every secret that it makes.`),
			"site": siteAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Datadog connector runs and stores the Datadog organization's admin keys`,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *DatadogAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatadogAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*datadogAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DatadogAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*datadogAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *DatadogAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*datadogAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &datadogAiCredentialStagedModel{
		Id:            types.StringValue(id),
		Site:          siteFromJson(jsonv.Site),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *DatadogAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*datadogAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := datadogAiCredentialStagedJson{
		Site: siteToJson(datav.Site),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *DatadogAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json datadogAiCredentialStagedApi
	var data datadogAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *DatadogAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &datadogAiCredentialStagedApi{}, &datadogAiCredentialStagedModel{})
}

func (r *DatadogAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json datadogAiCredentialStagedApi
	var data datadogAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *DatadogAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &datadogAiCredentialStagedModel{})
}

func (r *DatadogAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
