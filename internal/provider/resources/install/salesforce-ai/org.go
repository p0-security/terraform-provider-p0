package installsalesforceai

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

var _ resource.Resource = &SalesforceAiOrg{}
var _ resource.ResourceWithConfigure = &SalesforceAiOrg{}
var _ resource.ResourceWithImportState = &SalesforceAiOrg{}

type SalesforceAiOrg struct {
	installer *common.Install
}

type salesforceAiOrgModel struct {
	Id                     types.String                                 `tfsdk:"id"`
	FullDomain             types.String                                 `tfsdk:"full_domain"`
	SecretManager          *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ProvisionerConsumerKey types.String                                 `tfsdk:"provisioner_consumer_key"`
	AgentConsumerKey       types.String                                 `tfsdk:"agent_consumer_key"`
	State                  types.String                                 `tfsdk:"state"`
}

type salesforceAiOrgJson struct {
	FullDomain             *string                                     `json:"fullDomain,omitempty"`
	SecretManager          *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ProvisionerConsumerKey *string                                     `json:"provisionerConsumerKey,omitempty"`
	AgentConsumerKey       *string                                     `json:"agentClientId,omitempty"`
	State                  *string                                     `json:"state,omitempty"`
}

type salesforceAiOrgApi struct {
	Item *salesforceAiOrgJson `json:"item"`
}

func NewSalesforceAiOrg() resource.Resource {
	return &SalesforceAiOrg{}
}

func (*SalesforceAiOrg) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_salesforce_ai"
}

func (*SalesforceAiOrg) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Salesforce org installation for agentic access.

Installing it allows P0 to grant agents Salesforce permission sets on an API-only user for each gateway session.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_salesforce_ai_staged`" + `, deploy the connector's Cloud Run service, and add the P0 Provisioner app's consumer secret as a version of the connector's consumer secret secret. Creating this resource verifies that the connector can sign in to the org.

**Note:** This integration is currently in preview.

For the full install steps, see [Salesforce](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/salesforce).`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute("The `id` of the `p0_salesforce_ai_staged` resource being finalized"),
			"full_domain": fullDomainAttribute(" Must match the `p0_salesforce_ai_staged` resource."),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Salesforce connector runs and stores the P0 Provisioner app's consumer secret`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, " Must match the `p0_salesforce_ai_staged` resource."),
			},
			"provisioner_consumer_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The consumer key of the package's P0 Provisioner app, which the connector signs in with.`,
			},
			"agent_consumer_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The consumer key of the package's P0 Agent app, which the gateway exchanges its tokens with.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *SalesforceAiOrg) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  SalesforceAiKey,
		Component:    orgComponent,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *SalesforceAiOrg) getId(data any) *string {
	model, ok := data.(*salesforceAiOrgModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *SalesforceAiOrg) getItemJson(json any) any {
	inner, ok := json.(*salesforceAiOrgApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SalesforceAiOrg) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*salesforceAiOrgJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &salesforceAiOrgModel{
		Id:                     types.StringValue(id),
		FullDomain:             types.StringPointerValue(jsonv.FullDomain),
		SecretManager:          &secretManager,
		ProvisionerConsumerKey: types.StringPointerValue(jsonv.ProvisionerConsumerKey),
		AgentConsumerKey:       types.StringPointerValue(jsonv.AgentConsumerKey),
		State:                  types.StringPointerValue(jsonv.State),
	}
}

func (r *SalesforceAiOrg) toJson(data any) any {
	datav, ok := data.(*salesforceAiOrgModel)
	if !ok {
		return nil
	}

	json := salesforceAiOrgJson{
		FullDomain:             datav.FullDomain.ValueStringPointer(),
		ProvisionerConsumerKey: datav.ProvisionerConsumerKey.ValueStringPointer(),
		AgentConsumerKey:       datav.AgentConsumerKey.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *SalesforceAiOrg) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json salesforceAiOrgApi
	var data salesforceAiOrgModel

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

func (r *SalesforceAiOrg) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &salesforceAiOrgApi{}, &salesforceAiOrgModel{})
}

func (r *SalesforceAiOrg) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &salesforceAiOrgApi{}, &salesforceAiOrgModel{})
}

func (r *SalesforceAiOrg) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &salesforceAiOrgModel{})
}

func (r *SalesforceAiOrg) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
