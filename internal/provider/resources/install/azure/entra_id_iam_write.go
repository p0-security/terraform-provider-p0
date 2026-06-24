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
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &entraIdIamWrite{}
var _ resource.ResourceWithImportState = &entraIdIamWrite{}
var _ resource.ResourceWithConfigure = &entraIdIamWrite{}

func NewEntraIdIamWrite() resource.Resource {
	return &entraIdIamWrite{}
}

type entraIdIamWriteModel struct {
	TenantId         types.String `tfsdk:"tenant_id"`
	ClientId         types.String `tfsdk:"client_id"`
	SovereignCloudId types.String `tfsdk:"sovereign_cloud_id"`
	EmailField       types.String `tfsdk:"email_field"`
	Label            types.String `tfsdk:"label"`
	State            types.String `tfsdk:"state"`
}

type entraIdIamWriteJson struct {
	ClientId         string `json:"clientId"`
	SovereignCloudId string `json:"sovereignCloudId"`
	EmailField       string `json:"emailField"`
	Label            string `json:"label"`
	State            string `json:"state"`
}

type entraIdIamWriteApi struct {
	Item entraIdIamWriteJson `json:"item"`
}

type entraIdIamWrite struct {
	installer *common.Install
}

func (r *entraIdIamWrite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id_iam_write"
}

func (r *entraIdIamWrite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0 for IAM management of a Microsoft Entra ID (Azure AD) tenant.

To use this resource, you must also:
- create an App Registration in your Entra tenant for P0,
- expose an API with a ` + "`user_impersonation`" + ` scope on the App Registration,
- add Microsoft Graph application permissions and grant admin consent,
- create a federated identity credential on the App Registration pointing to P0's service account,
- deploy the P0 Security Perimeter as an Azure Function App in your tenant,
- configure App Service Authentication on the Function App using the App Registration,
- install the ` + "`p0_azure`" + ` resource.

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Microsoft Entra ID tenant (directory) ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Entra ID tenant ID must be a valid UUID"),
				},
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Application (client) ID of the App Registration created for P0 in this tenant.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Client ID must be a valid UUID"),
				},
			},
			"sovereign_cloud_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Azure sovereign cloud environment. Use `AzureCloud` for the global cloud or `AzureUSGovernment` for the US Government cloud.",
				Validators: []validator.String{
					stringvalidator.OneOf("AzureCloud", "AzureUSGovernment"),
				},
			},
			"email_field": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Entra user property used as each user's email address. One of `userPrincipalName`, `mail`, or `otherMails`.",
				Validators: []validator.String{
					stringvalidator.OneOf("userPrincipalName", "mail", "otherMails"),
				},
			},
			"label": labelAttribute,
			"state": common.StateAttribute,
		},
	}
}

func (r *entraIdIamWrite) getId(data any) *string {
	model, ok := data.(*entraIdIamWriteModel)
	if !ok {
		return nil
	}
	return model.TenantId.ValueStringPointer()
}

func (r *entraIdIamWrite) getItemJson(json any) any {
	inner, ok := json.(*entraIdIamWriteApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *entraIdIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := entraIdIamWriteModel{}
	jsonv, ok := json.(*entraIdIamWriteJson)
	if !ok {
		return nil
	}

	data.TenantId = types.StringValue(id)
	data.ClientId = types.StringValue(jsonv.ClientId)
	data.SovereignCloudId = types.StringValue(jsonv.SovereignCloudId)
	data.EmailField = types.StringValue(jsonv.EmailField)
	data.Label = types.StringValue(jsonv.Label)
	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *entraIdIamWrite) toJson(data any) any {
	model, ok := data.(*entraIdIamWriteModel)
	if !ok {
		return nil
	}
	return entraIdIamWriteJson{
		ClientId:         model.ClientId.ValueString(),
		SovereignCloudId: model.SovereignCloudId.ValueString(),
		EmailField:       model.EmailField.ValueString(),
	}
}

func (r *entraIdIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  EntraIdKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *entraIdIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraIdIamWriteApi
	var data entraIdIamWriteModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	if resp.Diagnostics.HasError() {
		return
	}
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *entraIdIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraIdIamWriteApi{}, &entraIdIamWriteModel{})
}

func (s *entraIdIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdIamWriteApi{}, &entraIdIamWriteModel{})
}

func (s *entraIdIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &entraIdIamWriteModel{})
}

func (s *entraIdIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
