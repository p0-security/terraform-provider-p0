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

var _ resource.Resource = &azureBastionHostStaged{}
var _ resource.ResourceWithImportState = &azureBastionHostStaged{}
var _ resource.ResourceWithConfigure = &azureBastionHostStaged{}

func NewAzureBastionHostStaged() resource.Resource {
	return &azureBastionHostStaged{}
}

type azureBastionHostStaged struct {
	installer *common.Install
}

type azureBastionHostStagedModel struct {
	SubscriptionId string       `tfsdk:"subscription_id"`
	State          types.String `tfsdk:"state"`
	CustomRole     types.Object `tfsdk:"custom_role"`
}

type azureBastionHostStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
	Metadata struct {
		CustomRole azureCustomRoleMetadata `json:"customRole"`
	} `json:"metadata"`
}

func (r *azureBastionHostStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_bastion_host_staged"
}

func (r *azureBastionHostStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Staged installation of the P0 Azure Bastion Host component. Returns the Bastion Host Management role spec so you can create the Azure role and assignment, then complete with ` + "`p0_azure_bastion_host`" + `.

To use this resource, you must also:
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,
- install the ` + "`p0_azure_iam_write`" + ` resource for the same subscription.

Read ` + "`custom_role`" + ` (name, description, actions, assignable_scope) when defining an ` + "`azurerm_role_definition`" + ` or equivalent, assign it to the P0 service principal, deploy Bastion, then pass the Bastion ARM ID and role definition ID to ` + "`p0_azure_bastion_host`" + `.
` + "\n\nExample:\n\n```terraform\n" +
			"resource \"p0_azure_bastion_host_staged\" \"example\" {\n" +
			"  depends_on = [\n" +
			"    p0_azure.example,\n" +
			"    p0_azure_app.example,\n" +
			"    p0_azure_iam_write.example,\n" +
			"  ]\n" +
			"\n" +
			"  subscription_id = local.subscription_id\n" +
			"}\n" +
			"```\n",
		Attributes: map[string]schema.Attribute{
			"subscription_id": subscriptionIdAttribute,
			"state":           common.StateAttribute,
			"custom_role": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The custom role spec for the P0 Bastion Host Management role.",
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
				},
			},
		},
	}
}

func (r *azureBastionHostStaged) getId(data any) *string {
	model, ok := data.(*azureBastionHostStagedModel)
	if !ok {
		return nil
	}
	return &model.SubscriptionId
}

func (r *azureBastionHostStaged) getItemJson(json any) any {
	return json
}

func (r *azureBastionHostStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureBastionHostStagedModel{}
	jsonv, ok := json.(*azureBastionHostStagedApi)
	if !ok {
		return nil
	}

	data.SubscriptionId = id
	data.State = types.StringValue(jsonv.Item.State)
	metadata := jsonv.Metadata
	actionsList, actionsDiags := types.ListValueFrom(ctx, types.StringType, metadata.CustomRole.Actions)
	if actionsDiags.HasError() {
		diags.Append(actionsDiags...)
		return nil
	}
	customRole, alDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"name":             types.StringType,
		"description":      types.StringType,
		"actions":          types.ListType{ElemType: types.StringType},
		"is_custom":        types.BoolType,
		"assignable_scope": types.StringType,
	}, map[string]attr.Value{
		"name":             types.StringValue(metadata.CustomRole.Name),
		"description":      types.StringValue(metadata.CustomRole.Description),
		"actions":          actionsList,
		"is_custom":        types.BoolValue(metadata.CustomRole.IsCustom),
		"assignable_scope": types.StringValue(metadata.CustomRole.AssignableScope),
	})
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.CustomRole = customRole

	return &data
}

func (r *azureBastionHostStaged) toJson(data any) any {
	return &struct{}{}
}

func (r *azureBastionHostStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    installresources.BastionHost,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *azureBastionHostStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json azureBastionHostStagedApi
	var data azureBastionHostStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
}

func (s *azureBastionHostStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &azureBastionHostStagedApi{}, &azureBastionHostStagedModel{})
}

func (s *azureBastionHostStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &azureBastionHostStagedModel{})
}

func (s *azureBastionHostStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &azureBastionHostStagedApi{}, &azureBastionHostStagedModel{})
}

func (s *azureBastionHostStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("subscription_id"), req, resp)
}
