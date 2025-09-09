package installazure

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AzureApp{}
var _ resource.ResourceWithImportState = &AzureApp{}
var _ resource.ResourceWithConfigure = &AzureApp{}

func NewAzureApp() resource.Resource {
	return &AzureApp{}
}

type AzureApp struct {
	installer *common.Install
}

type azureAppModel struct {
	ClientId                 types.String `tfsdk:"client_id"`
	ClientServicePrincipalId types.String `tfsdk:"client_service_principal_id"`
	State                    types.String `tfsdk:"state"`
}

type azureAppReqJson struct {
	ClientId string `json:"clientId"`
}

type azureAppJson struct {
	ClientId                 string `json:"clientId"`
	ClientServicePrincipalId string `json:"clientServicePrincipalId"`
	State                    string `json:"state"`
}

type azureAppJsonApi struct {
	Item azureAppJson `json:"item"`
}

func (r *AzureApp) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_app"
}

func (r *AzureApp) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Azure tenant.

For instructions on using this resource, see the documentation for ` + "`p0_azure_azure_app`.",
		Attributes: map[string]schema.Attribute{
			"state": common.StateAttribute,
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Microsoft Azure service account client ID.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Azure client ID must be a valid UUID"),
				},
			},
			"client_service_principal_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The Microsoft Azure service principal ID.`,
			},
		},
	}
}

func (r *AzureApp) getItemJson(json any) any {
	inner, ok := json.(*azureAppJsonApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *AzureApp) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureAppModel{}
	jsonv, ok := json.(*azureAppJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)
	data.ClientId = types.StringValue(jsonv.ClientId)

	return &data
}

func (r *AzureApp) toJson(data any) any {
	json := azureAppReqJson{}
	datav, ok := data.(*azureAppModel)
	if !ok {
		return nil
	}

	// can omit state here as it's filled by the backend
	json.ClientId = datav.ClientId.ValueString()
	return json
}

func (r *AzureApp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    AzureAppKey,
		ProviderData: providerData,
		GetId:        singletonGetId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AzureApp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json azureAppJsonApi
	var data azureAppModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *AzureApp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &azureAppJsonApi{}, &azureAppModel{})
}

func (s *AzureApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &azureAppModel{})
}

func (s *AzureApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &azureAppJsonApi{}, &azureAppModel{})
}

func (s *AzureApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Empty(), req, resp)
}
