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
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &AzureIamWriteStaged{}
var _ resource.ResourceWithImportState = &AzureIamWriteStaged{}
var _ resource.ResourceWithConfigure = &AzureIamWriteStaged{}

func NewAzureIamWriteStaged() resource.Resource {
	return &AzureIamWriteStaged{}
}

type AzureIamWriteStaged struct {
	installer *common.Install
}

type AzureIamWriteStagedModel struct {
	SubscriptionId string       `tfsdk:"subscription_id"`
	State          types.String `tfsdk:"state"`
	CustomRole     types.Object `tfsdk:"custom_role"`
}

type azureCustomRoleMetadata struct {
	Name            string   `json:"name" tfsdk:"name"`
	Description     string   `json:"description" tfsdk:"description"`
	Actions         []string `json:"actions" tfsdk:"actions"`
	IsCustom        bool     `json:"isCustom" tfsdk:"is_custom"`
	AssignableScope string   `json:"assignableScope" tfsdk:"assignable_scope"`
	Condition       string   `json:"condition" tfsdk:"condition"`
}

type AzureIamWriteStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
	Metadata struct {
		CustomRole azureCustomRoleMetadata `json:"customRole"`
	} `json:"metadata"`
}

func (r *AzureIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_iam_write_staged"
}

func (r *AzureIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Azure subscription, for IAM management.
		
To use this resource, you must also:
- create an app registration in Azure for P0,
- create federated credentials for P0 to communicate with Azure through the app registration,
- create a custom role allowing IAM operations,
- assign this custom role to P0's app registration at the subscription level,
- (optional) constraint role assignment to specific roles or principals,
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,

For instructions on using this resource, see the documentation for ` + "`p0_azure_iam_write`.",
		Attributes: map[string]schema.Attribute{
			"subscription_id": subscriptionIdAttribute,
			"state":           common.StateAttribute,
			"custom_role": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The custom role created for the Azure IAM Management.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The name of the Azure custom role.",
					},
					"description": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The description of the Azure custom role.",
					},
					"actions": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "The actions allowed for the Azure custom role.",
					},
					"is_custom": schema.BoolAttribute{
						Computed:            true,
						MarkdownDescription: "Indicates if the role is a custom role.",
					},
					"assignable_scope": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The assignable scope of the Azure custom role.",
					},
					"condition": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The condition of the Azure custom role assignment, if any.",
					},
				},
			},
		},
	}
}

func (r *AzureIamWriteStaged) getId(data any) *string {
	model, ok := data.(*AzureIamWriteStagedModel)
	if !ok {
		return nil
	}
	return &model.SubscriptionId
}

func (r *AzureIamWriteStaged) getItemJson(json any) any {
	return json
}

func (r *AzureIamWriteStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := AzureIamWriteStagedModel{}
	jsonv, ok := json.(*AzureIamWriteStagedApi)
	if !ok {
		return nil
	}

	data.SubscriptionId = id
	data.State = types.StringValue(jsonv.Item.State)
	metadata := jsonv.Metadata
	customRole, alDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"name":             types.StringType,
		"description":      types.StringType,
		"actions":          types.ListType{ElemType: types.StringType},
		"is_custom":        types.BoolType,
		"assignable_scope": types.StringType,
		"condition":        types.StringType,
	}, metadata.CustomRole)
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.CustomRole = customRole

	return &data
}

func (r *AzureIamWriteStaged) toJson(data any) any {
	json := AzureIamWriteStagedApi{}
	return &json.Item
}

func (r *AzureIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AzureIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json AzureIamWriteStagedApi
	var data AzureIamWriteStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
}

func (s *AzureIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &AzureIamWriteStagedApi{}, &AzureIamWriteStagedModel{})
}

func (s *AzureIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &AzureIamWriteStagedModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *AzureIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &AzureIamWriteStagedApi{}, &AzureIamWriteStagedModel{})
}

func (s *AzureIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("subscription_id"), req, resp)
}
