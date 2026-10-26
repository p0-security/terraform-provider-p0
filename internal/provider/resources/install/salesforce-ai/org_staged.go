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

var _ resource.Resource = &SalesforceAiOrgStaged{}
var _ resource.ResourceWithConfigure = &SalesforceAiOrgStaged{}
var _ resource.ResourceWithImportState = &SalesforceAiOrgStaged{}
var _ resource.ResourceWithValidateConfig = &SalesforceAiOrgStaged{}

type SalesforceAiOrgStaged struct {
	installer *common.Install
}

type salesforceAiOrgStagedModel struct {
	Id            types.String                                              `tfsdk:"id"`
	FullDomain    types.String                                              `tfsdk:"full_domain"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsStagedModel `tfsdk:"secret_manager"`
	State         types.String                                              `tfsdk:"state"`
}

type salesforceAiOrgStagedJson struct {
	FullDomain    *string                                            `json:"fullDomain,omitempty"`
	SecretManager *installvaultedcredential.SecretManagerWithAwsJson `json:"secretManager,omitempty"`
	State         *string                                            `json:"state,omitempty"`
}

type salesforceAiOrgStagedApi struct {
	Item *salesforceAiOrgStagedJson `json:"item"`
}

func NewSalesforceAiOrgStaged() resource.Resource {
	return &SalesforceAiOrgStaged{}
}

func (*SalesforceAiOrgStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_salesforce_ai_staged"
}

func (*SalesforceAiOrgStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Salesforce org installation for agentic access. Staging generates the identifiers needed to deploy P0's Salesforce connector.

Use the read-only ` + "`secret_manager`" + ` connector attributes to deploy the connector and its consumer secret's secret: a Lambda function with ` + "`aws-sm`" + `, or a Cloud Run service with ` + "`gcp-sm`" + `. Once the connector is deployed and the secret holds the consumer secret, create a ` + "`p0_salesforce_ai`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisites:** P0 must be installed on the AWS account with AWS Secrets Manager, or on the Google Cloud organization (for example via the ` + "`p0_gcp`" + ` resource), and the P0 agent access package must be set up in the org (see [p0-salesforce-agent-access](https://github.com/p0-security/p0-salesforce-agent-access)).

**Note:** This integration is currently in preview.

For the full install steps, see [Salesforce](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/salesforce).`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute(`An identifier for the Salesforce org, made of letters, digits and hyphens. Agents and policies name the org with it, and P0 puts it in the name of the consumer secret's secret.`),
			"full_domain": fullDomainAttribute(""),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Salesforce connector runs and stores the P0 Provisioner app's consumer secret`,
				Attributes:          installvaultedcredential.SecretManagerWithAwsAttributes(secretLabel, ""),
			},
			"state": common.StateAttribute,
		},
	}
}

func (*SalesforceAiOrgStaged) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	installvaultedcredential.ValidateSecretManagerWithAws(ctx, req.Config, &resp.Diagnostics)
}

func (r *SalesforceAiOrgStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SalesforceAiOrgStaged) getId(data any) *string {
	model, ok := data.(*salesforceAiOrgStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *SalesforceAiOrgStaged) getItemJson(json any) any {
	inner, ok := json.(*salesforceAiOrgStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *SalesforceAiOrgStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*salesforceAiOrgStagedJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManagerWithAws(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerWithAwsStagedFromJson(jsonv.SecretManager)
	return &salesforceAiOrgStagedModel{
		Id:            types.StringValue(id),
		FullDomain:    types.StringPointerValue(jsonv.FullDomain),
		SecretManager: &secretManager,
		State:         types.StringPointerValue(jsonv.State),
	}
}

func (r *SalesforceAiOrgStaged) toJson(data any) any {
	datav, ok := data.(*salesforceAiOrgStagedModel)
	if !ok {
		return nil
	}

	json := salesforceAiOrgStagedJson{
		FullDomain: datav.FullDomain.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *SalesforceAiOrgStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json salesforceAiOrgStagedApi
	var data salesforceAiOrgStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *SalesforceAiOrgStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &salesforceAiOrgStagedApi{}, &salesforceAiOrgStagedModel{})
}

func (r *SalesforceAiOrgStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json salesforceAiOrgStagedApi
	var data salesforceAiOrgStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *SalesforceAiOrgStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &salesforceAiOrgStagedModel{})
}

func (r *SalesforceAiOrgStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
