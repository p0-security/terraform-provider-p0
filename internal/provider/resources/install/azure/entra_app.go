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

var _ resource.Resource = &EntraApp{}
var _ resource.ResourceWithImportState = &EntraApp{}
var _ resource.ResourceWithConfigure = &EntraApp{}

func NewEntraApp() resource.Resource {
	return &EntraApp{}
}

type EntraApp struct {
	installer *common.Install
}

type entraAppModel struct {
	ClientId                 types.String `tfsdk:"client_id"`
	ClientServicePrincipalId types.String `tfsdk:"client_service_principal_id"`
	State                    types.String `tfsdk:"state"`
}

type entraAppReqJson struct {
	ClientId string `json:"clientId"`
}

type entraAppJson struct {
	ClientId                 string `json:"clientId"`
	ClientServicePrincipalId string `json:"clientServicePrincipalId"`
	State                    string `json:"state"`
}

type entraAppJsonApi struct {
	Item entraAppJson `json:"item"`
}

func (r *EntraApp) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_app"
}

func (r *EntraApp) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Registers the App Registration used by P0 to manage or assess a Microsoft Entra ID (Azure AD) tenant.

Create the Azure AD application and federated identity credential for P0 (see the example usage), then set ` + "`client_id`" + ` here to the new application's client ID. This resource must be installed before ` + "`p0_entra_id_iam_write`" + ` or ` + "`p0_entra_id_iam_assessment`" + `.`,
		Attributes: map[string]schema.Attribute{
			"state": common.StateAttribute,
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Application (client) ID of the App Registration created for P0.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Client ID must be a valid UUID"),
				},
			},
			"client_service_principal_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The Microsoft Entra ID service principal ID for the App Registration.`,
			},
		},
	}
}

func (r *EntraApp) getItemJson(json any) any {
	inner, ok := json.(*entraAppJsonApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *EntraApp) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := entraAppModel{}
	jsonv, ok := json.(*entraAppJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)
	data.ClientId = types.StringValue(jsonv.ClientId)
	data.ClientServicePrincipalId = types.StringValue(jsonv.ClientServicePrincipalId)

	return &data
}

func (r *EntraApp) toJson(data any) any {
	json := entraAppReqJson{}
	datav, ok := data.(*entraAppModel)
	if !ok {
		return nil
	}

	// can omit state here as it's filled by the backend
	json.ClientId = datav.ClientId.ValueString()
	return json
}

func (r *EntraApp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    EntraAppKey,
		ProviderData: providerData,
		GetId:        singletonGetId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *EntraApp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraAppJsonApi
	var data entraAppModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *EntraApp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraAppJsonApi{}, &entraAppModel{})
}

func (s *EntraApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &entraAppModel{})
}

func (s *EntraApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraAppJsonApi{}, &entraAppModel{})
}

func (s *EntraApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("client_id"), req, resp)
}
