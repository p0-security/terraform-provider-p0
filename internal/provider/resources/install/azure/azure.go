package installazure

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	DirectoryId         types.String `tfsdk:"directory_id"`
	State               types.String `tfsdk:"state"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	AppName             types.String `tfsdk:"app_name"`
	CredentialInfo      types.Object `tfsdk:"credential_info"`
}

type azureRequestApi struct {
	Root struct {
		Singleton struct {
			DirectoryId string `json:"directoryId"`
		} `json:"_"`
	} `json:"root"`
}

type azureCredentialMetadata struct {
	DisplayName string   `json:"name" tfsdk:"name"`
	Description string   `json:"description" tfsdk:"description"`
	Issuer      string   `json:"issuer" tfsdk:"issuer"`
	Audiences   []string `json:"audiences" tfsdk:"audiences"`
}

type azureApi struct {
	Config struct {
		Root struct {
			Singleton struct {
				DirectoryId         string  `json:"directoryId"`
				ServiceAccountEmail *string `json:"serviceAccountEmail"`
				ServiceAccountId    *string `json:"serviceAccountId"`
				State               string  `json:"state"`
			} `json:"_"`
		} `json:"root"`
	} `json:"config"`
	Metadata struct {
		AppName        string                  `json:"appName"`
		CredentialInfo azureCredentialMetadata `json:"credentialInfo"`
	} `json:"metadata"`
}

func (r *Azure) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure"
}

func (r *Azure) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Microsoft Azure installation.`,
		Attributes: map[string]schema.Attribute{
			"directory_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Microsoft Azure Directory ID`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Azure Directory ID must be a valid UUID"),
				},
			},
			"state": common.StateAttribute,
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The service identity email that P0 uses to communicate with your Microsoft Azure organization`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The service identity ID that P0 uses to communicate with your Microsoft Azure organization`,
			},
			"app_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the Azure application P0 uses to communicate with your Microsoft Azure organization. This name is used to identify the app in the Azure portal.",
			},
			"credential_info": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The credential information to setup Azure application federated credentials. This is used to authenticate P0 with your Microsoft Azure organization.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The display name of the Azure application federated credential.",
					},
					"description": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The description of the Azure application federated credential.",
					},
					"issuer": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The issuer of the Azure application federated credential.",
					},
					"audiences": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "The audience of the Azure application federated credential. This is used to establish a connection with the P0 service account",
					},
				},
			},
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

	data.DirectoryId = types.StringValue(root.DirectoryId)
	data.ServiceAccountEmail = types.StringPointerValue(root.ServiceAccountEmail)
	data.ServiceAccountId = types.StringPointerValue(root.ServiceAccountId)
	metadata := jsonv.Metadata

	data.AppName = types.StringValue(metadata.AppName)
	credentialInfo, alDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"issuer":      types.StringType,
		"audiences":   types.ListType{ElemType: types.StringType},
	}, metadata.CredentialInfo)
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.CredentialInfo = credentialInfo

	return &data
}

func (r *Azure) toJson(data any) any {
	json := azureRequestApi{}

	datav, ok := data.(*azureModel)
	if !ok {
		return nil
	}

	json.Root.Singleton.DirectoryId = datav.DirectoryId.ValueString()

	return &json
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
