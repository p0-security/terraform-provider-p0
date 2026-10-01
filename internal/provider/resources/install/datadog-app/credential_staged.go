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

var _ resource.Resource = &DatadogAppCredentialStaged{}
var _ resource.ResourceWithConfigure = &DatadogAppCredentialStaged{}
var _ resource.ResourceWithImportState = &DatadogAppCredentialStaged{}

type DatadogAppCredentialStaged struct {
	installer *common.Install
}

type datadogAppCredentialStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	Site          types.String                                       `tfsdk:"site"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type datadogAppCredentialStagedJson struct {
	Site          *siteJson                                   `json:"site,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type datadogAppCredentialStagedApi struct {
	Item *datadogAppCredentialStagedJson `json:"item"`
}

func NewDatadogAppCredentialStaged() resource.Resource {
	return &DatadogAppCredentialStaged{}
}

func (*DatadogAppCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_datadog_app_staged"
}

func (*DatadogAppCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Datadog installation for agentic access. Staging generates the identifiers needed to deploy P0's Datadog connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its admin keys secret. Once the connector is deployed, create a ` + "`p0_datadog_app`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is currently in preview.`,
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

func (r *DatadogAppCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatadogAppCredentialStaged) getId(data any) *string {
	model, ok := data.(*datadogAppCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *DatadogAppCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*datadogAppCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *DatadogAppCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*datadogAppCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &datadogAppCredentialStagedModel{
		Id:            types.StringValue(id),
		Site:          siteFromJson(jsonv.Site),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *DatadogAppCredentialStaged) toJson(data any) any {
	datav, ok := data.(*datadogAppCredentialStagedModel)
	if !ok {
		return nil
	}

	json := datadogAppCredentialStagedJson{
		Site: siteToJson(datav.Site),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *DatadogAppCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json datadogAppCredentialStagedApi
	var data datadogAppCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *DatadogAppCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &datadogAppCredentialStagedApi{}, &datadogAppCredentialStagedModel{})
}

func (r *DatadogAppCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json datadogAppCredentialStagedApi
	var data datadogAppCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *DatadogAppCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &datadogAppCredentialStagedModel{})
}

func (r *DatadogAppCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
