package entra_id

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &EntraIdIamWrite{}
var _ resource.ResourceWithImportState = &EntraIdIamWrite{}
var _ resource.ResourceWithConfigure = &EntraIdIamWrite{}

func NewEntraIdIamWrite() resource.Resource {
	return &EntraIdIamWrite{}
}

type EntraIdIamWrite struct {
	installer *common.Install
}

type EntraIdIamWriteModel struct {
	TenantId        types.String `tfsdk:"tenant_id"`
	ClientId        types.String `tfsdk:"client_id"`
	SovereignCloudId types.String `tfsdk:"sovereign_cloud_id"`
	EmailField      types.String `tfsdk:"email_field"`
	Label           types.String `tfsdk:"label"`
	State           types.String `tfsdk:"state"`
}

type entraIdIamWriteItemJson struct {
	State            string `json:"state"`
	Label            string `json:"label"`
	SovereignCloudId string `json:"sovereignCloudId"`
	ClientId         string `json:"clientId"`
	EmailField       string `json:"emailField"`
}

type entraIdIamWriteApi struct {
	Item entraIdIamWriteItemJson `json:"item"`
}

func (r *EntraIdIamWrite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id_iam_write"
}

func (r *EntraIdIamWrite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Entra ID (Azure AD) IAM write: assign users to Entra roles and add users to groups. Requires `p0_entra_id_iam_write_staged`, a caller app, and the Security Perimeter Function App to be created first.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": tenantIdAttribute,
			"client_id": schema.StringAttribute{
				Description: "Entra ID application (caller app) client ID.",
				Required:    true,
			},
			"sovereign_cloud_id": schema.StringAttribute{
				Description: "Sovereign cloud (e.g. AzureCloud, AzureUSGovernment).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("AzureCloud"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email_field": schema.StringAttribute{
				Description: "User property used for email (e.g. userPrincipalName, mail).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("userPrincipalName"),
			},
			"label":   labelAttribute,
			"state":   stateAttribute,
		},
	}
}

func (r *EntraIdIamWrite) getId(data any) *string {
	model, ok := data.(*EntraIdIamWriteModel)
	if !ok {
		return nil
	}
	return model.TenantId.ValueStringPointer()
}

func (r *EntraIdIamWrite) getItemJson(json any) any {
	inner, ok := json.(*entraIdIamWriteApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *EntraIdIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*entraIdIamWriteItemJson)
	if !ok {
		return nil
	}
	return &EntraIdIamWriteModel{
		TenantId:         types.StringValue(id),
		ClientId:         types.StringValue(item.ClientId),
		SovereignCloudId:  types.StringValue(item.SovereignCloudId),
		EmailField:       types.StringValue(item.EmailField),
		Label:            types.StringValue(item.Label),
		State:            types.StringValue(item.State),
	}
}

func (r *EntraIdIamWrite) toJson(data any) any {
	model, ok := data.(*EntraIdIamWriteModel)
	if !ok {
		return nil
	}
	return &entraIdIamWriteItemJson{
		ClientId:         model.ClientId.ValueString(),
		SovereignCloudId: model.SovereignCloudId.ValueString(),
		EmailField:       model.EmailField.ValueString(),
	}
}

func (r *EntraIdIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  installresources.EntraIdKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *EntraIdIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		resp.Diagnostics.AddError("Provider not configured", "Install is not configured; configure the provider before creating the resource.")
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdIamWriteApi{}, &EntraIdIamWriteModel{})
}

func (r *EntraIdIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		return
	}
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraIdIamWriteApi{}, &EntraIdIamWriteModel{})
}

func (r *EntraIdIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		resp.Diagnostics.AddError("Provider not configured", "Install is not configured; configure the provider before updating the resource.")
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdIamWriteApi{}, &EntraIdIamWriteModel{})
}

func (r *EntraIdIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		return
	}
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &EntraIdIamWriteModel{})
}

func (r *EntraIdIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
