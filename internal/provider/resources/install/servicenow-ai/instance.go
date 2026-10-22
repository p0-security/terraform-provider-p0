package installservicenowai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &ServiceNowAiInstance{}
var _ resource.ResourceWithConfigure = &ServiceNowAiInstance{}
var _ resource.ResourceWithImportState = &ServiceNowAiInstance{}

type ServiceNowAiInstance struct {
	installer *common.Install
}

type serviceNowAiInstanceModel struct {
	Id                  types.String                                 `tfsdk:"id"`
	SecretManager       *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ProvisionerClientId types.String                                 `tfsdk:"provisioner_client_id"`
	JwtClientId         types.String                                 `tfsdk:"jwt_client_id"`
	KeyId               types.String                                 `tfsdk:"key_id"`
	State               types.String                                 `tfsdk:"state"`
}

type serviceNowAiInstanceJson struct {
	SecretManager       *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ProvisionerClientId *string                                     `json:"provisionerClientId,omitempty"`
	JwtClientId         *string                                     `json:"jwtClientId,omitempty"`
	KeyId               *string                                     `json:"keyId,omitempty"`
	State               *string                                     `json:"state,omitempty"`
}

type serviceNowAiInstanceApi struct {
	Item *serviceNowAiInstanceJson `json:"item"`
}

func NewServiceNowAiInstance() resource.Resource {
	return &ServiceNowAiInstance{}
}

func (*ServiceNowAiInstance) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_servicenow_ai"
}

func (*ServiceNowAiInstance) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A ServiceNow instance installation for agentic access.

Installing it allows P0 to grant agents ServiceNow roles on a machine user for each gateway session.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_servicenow_ai_staged`" + `, deploy the connector's Cloud Run service, and add the client secrets and signing key as a version of the connector's install secret. Creating this resource verifies that the connector can sign in to the instance and read its roles.

**Note:** This integration is currently in preview.

For the full install steps, see [ServiceNow](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/servicenow).`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute("The `id` of the `p0_servicenow_ai_staged` resource being finalized"),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's ServiceNow connector runs and stores its client secrets, signing key, and session tokens`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_servicenow_ai_staged` resource."),
			},
			"provisioner_client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID of the client credential integration whose OAuth application user has only ` + "`user_admin`" + `. The connector signs in with it to manage agent users.`,
			},
			"jwt_client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The client ID of the JWT bearer integration that trusts the connector's signing certificate. The connector gets each agent user's token with it.`,
			},
			"key_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ` + "`kid`" + ` of the signing certificate in the JWT bearer integration's verifier map`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *ServiceNowAiInstance) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  ServiceNowAiKey,
		Component:    instanceComponent,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *ServiceNowAiInstance) getId(data any) *string {
	model, ok := data.(*serviceNowAiInstanceModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *ServiceNowAiInstance) getItemJson(json any) any {
	inner, ok := json.(*serviceNowAiInstanceApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *ServiceNowAiInstance) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*serviceNowAiInstanceJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &serviceNowAiInstanceModel{
		Id:                  types.StringValue(id),
		SecretManager:       &secretManager,
		ProvisionerClientId: types.StringPointerValue(jsonv.ProvisionerClientId),
		JwtClientId:         types.StringPointerValue(jsonv.JwtClientId),
		KeyId:               types.StringPointerValue(jsonv.KeyId),
		State:               types.StringPointerValue(jsonv.State),
	}
}

func (r *ServiceNowAiInstance) toJson(data any) any {
	datav, ok := data.(*serviceNowAiInstanceModel)
	if !ok {
		return nil
	}

	json := serviceNowAiInstanceJson{
		ProvisionerClientId: datav.ProvisionerClientId.ValueStringPointer(),
		JwtClientId:         datav.JwtClientId.ValueStringPointer(),
		KeyId:               datav.KeyId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *ServiceNowAiInstance) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json serviceNowAiInstanceApi
	var data serviceNowAiInstanceModel

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

func (r *ServiceNowAiInstance) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &serviceNowAiInstanceApi{}, &serviceNowAiInstanceModel{})
}

func (r *ServiceNowAiInstance) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &serviceNowAiInstanceApi{}, &serviceNowAiInstanceModel{})
}

func (r *ServiceNowAiInstance) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &serviceNowAiInstanceModel{})
}

func (r *ServiceNowAiInstance) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
