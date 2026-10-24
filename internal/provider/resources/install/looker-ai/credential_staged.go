package installlookerai

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

var _ resource.Resource = &LookerAiCredentialStaged{}
var _ resource.ResourceWithConfigure = &LookerAiCredentialStaged{}
var _ resource.ResourceWithImportState = &LookerAiCredentialStaged{}

type LookerAiCredentialStaged struct {
	installer *common.Install
}

type lookerAiCredentialStagedModel struct {
	Id             types.String                                       `tfsdk:"id"`
	InstanceUrl    types.String                                       `tfsdk:"instance_url"`
	AdminKeySecret types.String                                       `tfsdk:"admin_key_secret"`
	SecretManager  *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State          types.String                                       `tfsdk:"state"`
}

type lookerAiCredentialStagedJson struct {
	InstanceUrl    *string                                     `json:"instanceUrl,omitempty"`
	AdminKeySecret *string                                     `json:"adminKeySecret,omitempty"`
	SecretManager  *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State          *string                                     `json:"state,omitempty"`
}

type lookerAiCredentialStagedApi struct {
	Item *lookerAiCredentialStagedJson `json:"item"`
}

func NewLookerAiCredentialStaged() resource.Resource {
	return &LookerAiCredentialStaged{}
}

func (*LookerAiCredentialStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_looker_ai_staged"
}

func (*LookerAiCredentialStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Looker installation for agentic access. Staging gives you the names to deploy P0's Looker connector with.

Deploy the connector's Cloud Run service from the read-only ` + "`secret_manager`" + ` connector attributes, and let it read the Admin key's secret. Once the connector runs and the Admin key is in its secret, create a ` + "`p0_looker_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource).

**Note:** This integration is in preview.

For the full install steps, see [Looker](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/looker).`,
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(`The name P0 gives the Looker instance, made of letters, digits and hyphens. Agents, policies and the CLI refer to the instance by it, and P0 puts it in the name of every secret it makes.`),
			"instance_url":     instanceUrlAttribute(""),
			"admin_key_secret": adminKeySecretAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Looker connector runs and where the Looker instance's Admin key is stored`,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *LookerAiCredentialStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  LookerAiKey,
		Component:    installresources.Credential,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *LookerAiCredentialStaged) getId(data any) *string {
	model, ok := data.(*lookerAiCredentialStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *LookerAiCredentialStaged) getItemJson(json any) any {
	inner, ok := json.(*lookerAiCredentialStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *LookerAiCredentialStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*lookerAiCredentialStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &lookerAiCredentialStagedModel{
		Id:             types.StringValue(id),
		InstanceUrl:    types.StringPointerValue(jsonv.InstanceUrl),
		AdminKeySecret: types.StringPointerValue(jsonv.AdminKeySecret),
		SecretManager:  &secretManager,
		State:          types.StringPointerValue(jsonv.State),
	}
}

func (r *LookerAiCredentialStaged) toJson(data any) any {
	datav, ok := data.(*lookerAiCredentialStagedModel)
	if !ok {
		return nil
	}

	json := lookerAiCredentialStagedJson{
		InstanceUrl:    datav.InstanceUrl.ValueStringPointer(),
		AdminKeySecret: datav.AdminKeySecret.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *LookerAiCredentialStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json lookerAiCredentialStagedApi
	var data lookerAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *LookerAiCredentialStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &lookerAiCredentialStagedApi{}, &lookerAiCredentialStagedModel{})
}

func (r *LookerAiCredentialStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json lookerAiCredentialStagedApi
	var data lookerAiCredentialStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *LookerAiCredentialStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &lookerAiCredentialStagedModel{})
}

func (r *LookerAiCredentialStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
