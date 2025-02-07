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

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Azure{}
var _ resource.ResourceWithImportState = &Azure{}
var _ resource.ResourceWithConfigure = &Azure{}

func NewAzure() resource.Resource {
	return &Azure{}
}

type Azure struct {
	installer *common.RootInstall
}

type azureModel struct {
	ClientId            types.String `tfsdk:"client_id"`
	DirectoryId         types.String `tfsdk:"tenant_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	State               types.String `tfsdk:"state"`
}

type azureApi struct {
	Config azureConfig `json:"config"`
}

type azureConfig struct {
	Root azureRoot `json:"root"`
}

type azureRoot struct {
	Singleton azureSingleton `json:"_"`
}

type azureSingleton struct {
	ClientId            *string `json:"clientId"`
	DirectoryId         *string `json:"directoryId"`
	ServiceAccountEmail *string `json:"serviceAccountEmail,omitempty"`
	ServiceAccountId    *string `json:"serviceAccountId,omitempty"`
	State               *string `json:"state,omitempty"`
}

func (r *Azure) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure"
}

func (r *Azure) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Microsoft Azure installation.`,
		Attributes: map[string]schema.Attribute{
			// Azure tenant_id is also called directory_id, in our backend we call it directory_id.
			// We call it tenant_id here to match the Azure Terraform Provider.
			"tenant_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Microsoft Azure Directory ID`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Azure Directory ID must be a valid UUID"),
				},
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Microsoft Azure Service Account Client ID.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Azure Client ID must be a valid UUID"),
				},
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The identity that P0 uses to communicate with your Microsoft Azure organization`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The ID of the service account that P0 uses to communicate with your Microsoft Azure organization`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *Azure) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.RootInstall{
		Integration:  AzureKey,
		ProviderData: providerData,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *Azure) fromJson(ctx context.Context, diags *diag.Diagnostics, json any) any {
	data := azureModel{}

	jsonv, ok := json.(*azureApi)
	if !ok {
		return nil
	}

	root := jsonv.Config.Root.Singleton

	data.DirectoryId = types.StringPointerValue(root.DirectoryId)
	data.ClientId = types.StringPointerValue(root.ClientId)
	data.ServiceAccountEmail = types.StringPointerValue(root.ServiceAccountEmail)
	data.ServiceAccountId = types.StringPointerValue(root.ServiceAccountId)
	data.State = types.StringPointerValue(jsonv.Config.Root.Singleton.State)

	return &data
}

func (r *Azure) toJson(data any) any {
	json := azureApi{}

	datav, ok := data.(*azureModel)
	if !ok {
		return nil
	}

	json.Config.Root.Singleton.DirectoryId = datav.DirectoryId.ValueStringPointer()
	json.Config.Root.Singleton.ClientId = datav.ClientId.ValueStringPointer()

	return &json.Config
}

func (r *Azure) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json azureApi
	var data azureModel
	r.installer.Create(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Azure) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json azureApi
	var data azureModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *Azure) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cannot Update", "Modifying P0's Azure integration forces replacement")
}

func (r *Azure) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data azureModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *Azure) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("directory_id"), req, resp)
}
