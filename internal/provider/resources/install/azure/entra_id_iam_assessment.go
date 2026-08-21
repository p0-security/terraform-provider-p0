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

var _ resource.Resource = &entraIdIamAssessment{}
var _ resource.ResourceWithImportState = &entraIdIamAssessment{}
var _ resource.ResourceWithConfigure = &entraIdIamAssessment{}

func NewEntraIdIamAssessment() resource.Resource {
	return &entraIdIamAssessment{}
}

type entraIdIamAssessmentModel struct {
	TenantId         types.String `tfsdk:"tenant_id"`
	ClientId         types.String `tfsdk:"client_id"`
	SovereignCloudId types.String `tfsdk:"sovereign_cloud_id"`
	EmailField       types.String `tfsdk:"email_field"`
	Label            types.String `tfsdk:"label"`
	State            types.String `tfsdk:"state"`
}

type entraIdIamAssessmentJson struct {
	ClientId         string `json:"clientId"`
	SovereignCloudId string `json:"sovereignCloudId"`
	EmailField       string `json:"emailField"`
	Label            string `json:"label"`
	State            string `json:"state"`
}

type entraIdIamAssessmentApi struct {
	Item entraIdIamAssessmentJson `json:"item"`
}

type entraIdIamAssessment struct {
	installer *common.Install
}

func (r *entraIdIamAssessment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id_iam_assessment"
}

func (r *entraIdIamAssessment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0 for read-only IAM assessment of a Microsoft Entra ID (Azure AD) tenant.

This resource is read-only: it does not write role assignments back to Entra ID, and does not require a Function App or the P0 Security Perimeter. To use this resource, you must also:
- install the ` + "`p0_azure`" + ` resource,
- create an App Registration in your Entra tenant for P0 and register it with ` + "`p0_entra_app`" + `,
- add the Microsoft Graph application permissions required for read-only assessment and grant admin consent,
- create a federated identity credential on the App Registration pointing to P0's service account.

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

func (r *entraIdIamAssessment) getId(data any) *string {
	model, ok := data.(*entraIdIamAssessmentModel)
	if !ok {
		return nil
	}
	return model.TenantId.ValueStringPointer()
}

func (r *entraIdIamAssessment) getItemJson(json any) any {
	inner, ok := json.(*entraIdIamAssessmentApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *entraIdIamAssessment) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := entraIdIamAssessmentModel{}
	jsonv, ok := json.(*entraIdIamAssessmentJson)
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

func (r *entraIdIamAssessment) toJson(data any) any {
	model, ok := data.(*entraIdIamAssessmentModel)
	if !ok {
		return nil
	}
	return entraIdIamAssessmentJson{
		ClientId:         model.ClientId.ValueString(),
		SovereignCloudId: model.SovereignCloudId.ValueString(),
		EmailField:       model.EmailField.ValueString(),
	}
}

func (r *entraIdIamAssessment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  EntraIdIamAssessmentKey,
		Component:    installresources.IamAssessment,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *entraIdIamAssessment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraIdIamAssessmentApi
	var data entraIdIamAssessmentModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	if resp.Diagnostics.HasError() {
		return
	}
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *entraIdIamAssessment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraIdIamAssessmentApi{}, &entraIdIamAssessmentModel{})
}

func (s *entraIdIamAssessment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdIamAssessmentApi{}, &entraIdIamAssessmentModel{})
}

func (s *entraIdIamAssessment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &entraIdIamAssessmentModel{})
}

func (s *entraIdIamAssessment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
