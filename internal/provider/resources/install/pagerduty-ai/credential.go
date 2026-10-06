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

var _ resource.Resource = &PagerdutyAiCredential{}
var _ resource.ResourceWithConfigure = &PagerdutyAiCredential{}
var _ resource.ResourceWithImportState = &PagerdutyAiCredential{}

type PagerdutyAiCredential struct {
	installer *common.Install
}

type pagerdutyAiCredentialModel struct {
	Id            types.String                                 `tfsdk:"id"`
	Region        types.String                                 `tfsdk:"region"`
	SecretManager *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ClientId      types.String                                 `tfsdk:"client_id"`
	State         types.String                                 `tfsdk:"state"`
}

type pagerdutyAiCredentialJson struct {
	Region        *regionJson                                 `json:"region,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ClientId      *string                                     `json:"clientId,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type pagerdutyAiCredentialApi struct {
	Item *pagerdutyAiCredentialJson `json:"item"`
}

func NewPagerdutyAiCredential() resource.Resource {
	return &PagerdutyAiCredential{}
}

func (*PagerdutyAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_pagerduty_ai"
}

func (*PagerdutyAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A PagerDuty installation for agentic access.

Installing it allows P0 to grant machines just-in-time access to a PagerDuty account, with an app token that holds only the requested scopes.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_pagerduty_ai_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is deployed. After you create it, add the PagerDuty app's client secret as a version of the connector's client secret secret; the connector cannot request tokens until that version exists.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id":        idAttribute("The `id` of the `p0_pagerduty_ai_staged` resource being finalized"),
			"region":    regionAttribute(" Must match the `p0_pagerduty_ai_staged` resource."),
			"subdomain": subdomainAttribute(" Must match the `p0_pagerduty_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's PagerDuty connector runs and stores the PagerDuty app's client secret`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_pagerduty_ai_staged` resource."),
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID of the PagerDuty Scoped OAuth app that P0 requests tokens with. The app's scopes limit which scopes can be requested; it must also hold ` + "`users.read`" + `.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *PagerdutyAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PagerdutyAiCredential) getId(data any) *string {
	model, ok := data.(*pagerdutyAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *PagerdutyAiCredential) getItemJson(json any) any {
	inner, ok := json.(*pagerdutyAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *PagerdutyAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*pagerdutyAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &pagerdutyAiCredentialModel{
		Id:            types.StringValue(id),
		Region:        regionFromJson(jsonv.Region),
		SecretManager: &secretManager,
		ClientId:      types.StringPointerValue(jsonv.ClientId),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *PagerdutyAiCredential) toJson(data any) any {
	datav, ok := data.(*pagerdutyAiCredentialModel)
	if !ok {
		return nil
	}

	json := pagerdutyAiCredentialJson{
		Region:   regionToJson(datav.Region),
		ClientId: datav.ClientId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *PagerdutyAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json pagerdutyAiCredentialApi
	var data pagerdutyAiCredentialModel

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

func (r *PagerdutyAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &pagerdutyAiCredentialApi{}, &pagerdutyAiCredentialModel{})
}

func (r *PagerdutyAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pagerdutyAiCredentialApi{}, &pagerdutyAiCredentialModel{})
}

func (r *PagerdutyAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &pagerdutyAiCredentialModel{})
}

func (r *PagerdutyAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
