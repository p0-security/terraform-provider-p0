package installazure

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AzureAppStaged{}
var _ resource.ResourceWithImportState = &AzureAppStaged{}
var _ resource.ResourceWithConfigure = &AzureAppStaged{}

func NewAzureAppStaged() resource.Resource {
	return &AzureAppStaged{}
}

type AzureAppStaged struct {
	installer *common.Install
}

type azureAppStagedModel struct {
	State          types.String `tfsdk:"state"`
	AppName        types.String `tfsdk:"app_name"`
	CredentialInfo types.Object `tfsdk:"credential_info"`
}

type azureCredentialInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Issuer      string   `json:"issuer"`
	Audiences   []string `json:"audiences"`
}

type azureAppStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
	Metadata struct {
		AppName        string              `json:"appName"`
		CredentialInfo azureCredentialInfo `json:"credentialInfo"`
	} `json:"metadata"`
}

func (r *AzureAppStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_app_staged"
}

func (r *AzureAppStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Staged installation of the P0 Azure App Registration. After apply, read ` + "`app_name`" + ` and ` + "`credential_info`" + ` to create the Azure AD application and federated identity credential, then complete installation with ` + "`p0_azure_app`" + ` using the new app's client ID.

Use ` + "`p0_azure.example.service_account_id`" + ` as the federated credential ` + "`subject`" + ` (see ` + "`credential_info`" + ` for issuer, audiences, and suggested name/description).

To use this resource, you must first install the ` + "`p0_azure`" + ` resource.
` + "\n\nExample:\n\n```terraform\n" +
			"resource \"p0_azure_app_staged\" \"example\" {\n" +
			"  depends_on = [p0_azure.example]\n" +
			"}\n" +
			"\n" +
			"resource \"azuread_application_registration\" \"p0\" {\n" +
			"  display_name = p0_azure_app_staged.example.app_name\n" +
			"}\n" +
			"\n" +
			"resource \"azuread_application_federated_identity_credential\" \"p0\" {\n" +
			"  application_id = azuread_application_registration.p0.id\n" +
			"  display_name   = p0_azure_app_staged.example.credential_info.name\n" +
			"  description    = p0_azure_app_staged.example.credential_info.description\n" +
			"  issuer         = p0_azure_app_staged.example.credential_info.issuer\n" +
			"  audiences      = p0_azure_app_staged.example.credential_info.audiences\n" +
			"  subject        = p0_azure.example.service_account_id\n" +
			"}\n" +
			"\n" +
			"resource \"p0_azure_app\" \"example\" {\n" +
			"  depends_on = [azuread_application_federated_identity_credential.p0]\n" +
			"  client_id  = azuread_application_registration.p0.client_id\n" +
			"}\n" +
			"```\n",
		Attributes: map[string]schema.Attribute{
			"state": common.StateAttribute,
			"app_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The display name to use when creating the Azure AD app registration.",
			},
			"credential_info": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Federated identity credential parameters for the Azure AD app (issuer, audiences, etc.). Use with p0_azure root service_account_id as subject.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Display name for the federated credential.",
					},
					"description": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Description for the federated credential.",
					},
					"issuer": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Issuer URL for the federated credential.",
					},
					"audiences": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Audiences for the federated credential.",
					},
				},
			},
		},
	}
}

func (r *AzureAppStaged) getId(data any) *string {
	k := common.SingletonKey
	return &k
}

func (r *AzureAppStaged) getItemJson(json any) any {
	return json
}

func (r *AzureAppStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureAppStagedModel{}
	jsonv, ok := json.(*azureAppStagedApi)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.Item.State)
	data.AppName = types.StringValue(jsonv.Metadata.AppName)
	cred := jsonv.Metadata.CredentialInfo
	audiencesList, audiencesDiags := types.ListValueFrom(ctx, types.StringType, cred.Audiences)
	if audiencesDiags.HasError() {
		diags.Append(audiencesDiags...)
		return nil
	}
	credObj, alDiags := types.ObjectValue(
		map[string]attr.Type{
			"name":        types.StringType,
			"description": types.StringType,
			"issuer":      types.StringType,
			"audiences":   types.ListType{ElemType: types.StringType},
		},
		map[string]attr.Value{
			"name":        types.StringValue(cred.Name),
			"description": types.StringValue(cred.Description),
			"issuer":      types.StringValue(cred.Issuer),
			"audiences":   audiencesList,
		},
	)
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.CredentialInfo = credObj

	return &data
}

func (r *AzureAppStaged) toJson(data any) any {
	return &struct{}{}
}

func (r *AzureAppStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    AzureAppKey,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AzureAppStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json azureAppStagedApi
	var data azureAppStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
}

func (s *AzureAppStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &azureAppStagedApi{}, &azureAppStagedModel{})
}

func (s *AzureAppStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &azureAppStagedModel{})
}

func (s *AzureAppStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &azureAppStagedApi{}, &azureAppStagedModel{})
}

func (s *AzureAppStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Empty(), req, resp)
}
