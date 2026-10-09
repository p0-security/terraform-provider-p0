package installpagerdutyai

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

var _ resource.Resource = &PagerdutyAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &PagerdutyAiCredentialStaged{}
var _ resource.ResourceWithImportState = &PagerdutyAiCredentialStaged{}

type PagerdutyAiCredentialStaged struct {
	installer *common.Install
}

type pagerdutyAiCredentialStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	Region        types.String                                       `tfsdk:"region"`
	Subdomain     types.String                                       `tfsdk:"subdomain"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type pagerdutyAiCredentialStagedJson struct {
	Region        *regionJson                                 `json:"region,omitempty"`
	Subdomain     *string                                     `json:"subdomain,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type pagerdutyAiCredentialStagedApi struct {
	Item *pagerdutyAiCredentialStagedJson `json:"item"`
}

func NewPagerdutyAiCredentialStaged() resource.Resource {
	return &PagerdutyAiCredentialStaged{}
}

func (*PagerdutyAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_pagerduty_ai_staged"
}

func (*PagerdutyAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged PagerDuty installation for agentic access. Staging generates the identifiers needed to deploy P0's PagerDuty connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its client secret's secret. Once the connector is deployed, create a ` + "`p0_pagerduty_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

` + common.NotePreview,
		Attributes: map[string]schema.Attribute{
			"id":        idAttribute(`An identifier for the PagerDuty account, made of letters, digits and hyphens. P0 puts it in the name of every secret that it makes.`),
			"region":    regionAttribute(""),
			"subdomain": subdomainAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's PagerDuty connector runs and stores the PagerDuty app's client secret`,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *PagerdutyAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  PagerdutyAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *PagerdutyAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*pagerdutyAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *PagerdutyAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*pagerdutyAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *PagerdutyAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*pagerdutyAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &pagerdutyAiCredentialStagedModel{
		Id:            types.StringValue(id),
		Region:        regionFromJson(jsonv.Region),
		Subdomain:     types.StringPointerValue(jsonv.Subdomain),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *PagerdutyAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*pagerdutyAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := pagerdutyAiCredentialStagedJson{
		Region:    regionToJson(datav.Region),
		Subdomain: datav.Subdomain.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *PagerdutyAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json pagerdutyAiCredentialStagedApi
	var data pagerdutyAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *PagerdutyAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &pagerdutyAiCredentialStagedApi{}, &pagerdutyAiCredentialStagedModel{})
}

func (r *PagerdutyAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json pagerdutyAiCredentialStagedApi
	var data pagerdutyAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *PagerdutyAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &pagerdutyAiCredentialStagedModel{})
}

func (r *PagerdutyAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
