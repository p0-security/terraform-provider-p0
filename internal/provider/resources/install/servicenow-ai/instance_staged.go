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

var _ resource.Resource = &ServiceNowAiInstanceStaged{}
var _ resource.ResourceWithConfigure = &ServiceNowAiInstanceStaged{}
var _ resource.ResourceWithImportState = &ServiceNowAiInstanceStaged{}

type ServiceNowAiInstanceStaged struct {
	installer *common.Install
}

type serviceNowAiInstanceStagedModel struct {
	Id            types.String                                       `tfsdk:"id"`
	SecretManager *installvaultedcredential.SecretManagerStagedModel `tfsdk:"secret_manager"`
	State         types.String                                       `tfsdk:"state"`
}

type serviceNowAiInstanceStagedJson struct {
	SecretManager *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	State         *string                                     `json:"state,omitempty"`
}

type serviceNowAiInstanceStagedApi struct {
	Item *serviceNowAiInstanceStagedJson `json:"item"`
}

func NewServiceNowAiInstanceStaged() resource.Resource {
	return &ServiceNowAiInstanceStaged{}
}

func (*ServiceNowAiInstanceStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_servicenow_ai_staged"
}

func (*ServiceNowAiInstanceStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged ServiceNow instance installation for agentic access. Staging generates the identifiers needed to deploy P0's ServiceNow connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector's Cloud Run service and its install secret. Once the connector is deployed and the install secret holds the client secrets and signing key, create a ` + "`p0_servicenow_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisites:** P0 must be installed on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource), and the instance must have the provisioner and JWT bearer inbound integrations set up.

**Note:** This integration is currently in preview.

For the full install steps, see [ServiceNow](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/servicenow).`,
		Attributes: map[string]schema.Attribute{
			"id": idAttribute(`The instance name, as in ` + "`<name>.service-now.com`" + `. Agents and policies name the instance with it, and P0 puts it in the names of the connector's secrets.`),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's ServiceNow connector runs and stores its client secrets, signing key, and session tokens`,
				Attributes:          installvaultedcredential.SecretManagerAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *ServiceNowAiInstanceStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceNowAiInstanceStaged) getId(data any) *string {
	model, ok := data.(*serviceNowAiInstanceStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *ServiceNowAiInstanceStaged) getItemJson(json any) any {
	inner, ok := json.(*serviceNowAiInstanceStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *ServiceNowAiInstanceStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*serviceNowAiInstanceStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerStagedFromJson(jsonv.SecretManager)
	return &serviceNowAiInstanceStagedModel{
		Id:            types.StringValue(id),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *ServiceNowAiInstanceStaged) toJson(data any) any {
	datav, ok := data.(*serviceNowAiInstanceStagedModel)
	if !ok {
		return nil
	}

	json := serviceNowAiInstanceStagedJson{}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *ServiceNowAiInstanceStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json serviceNowAiInstanceStagedApi
	var data serviceNowAiInstanceStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *ServiceNowAiInstanceStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &serviceNowAiInstanceStagedApi{}, &serviceNowAiInstanceStagedModel{})
}

func (r *ServiceNowAiInstanceStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json serviceNowAiInstanceStagedApi
	var data serviceNowAiInstanceStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *ServiceNowAiInstanceStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &serviceNowAiInstanceStagedModel{})
}

func (r *ServiceNowAiInstanceStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
