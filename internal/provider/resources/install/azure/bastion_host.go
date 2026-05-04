// Azure Bastion host registration wires an existing Microsoft Azure Bastion into P0 for
// subscription-scoped SSH. Stage with `p0_azure_bastion_host_staged`, create the role and Bastion in
// Azure (for example with the `azure_p0_roles` and `azure_p0_bastion` modules), then pass the Bastion
// ARM ID and role definition ID into this resource. See `examples/resources/p0_azure_bastion_host/`.

package installazure

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &azureBastionHost{}
var _ resource.ResourceWithImportState = &azureBastionHost{}
var _ resource.ResourceWithConfigure = &azureBastionHost{}

func NewAzureBastionHost() resource.Resource {
	return &azureBastionHost{}
}

type azureBastionHost struct {
	installer *common.Install
}

type azureBastionHostModel struct {
	SubscriptionId   types.String `tfsdk:"subscription_id"`
	BastionId        string       `tfsdk:"bastion_id"`
	RoleDefinitionId types.String `tfsdk:"role_definition_id"`
	Label            types.String `tfsdk:"label"`
	State            types.String `tfsdk:"state"`
}

// Item request/response for the P0 API (camelCase for API).
type bastionHostBastionRef struct {
	Type      string `json:"type"`
	BastionId string `json:"bastionId,omitempty"`
}

type bastionHostItemJson struct {
	Bastion          bastionHostBastionRef `json:"bastion"`
	RoleDefinitionId string                `json:"roleDefinitionId"`
	State            string                `json:"state,omitempty"`
	Label            string                `json:"label,omitempty"`
}

type bastionHostApi struct {
	Item bastionHostItemJson `json:"item"`
}

func (r *azureBastionHost) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_bastion_host"
}

func (r *azureBastionHost) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Registers an Azure Bastion host with P0 for SSH access to VMs in a subscription.

To use this resource, you must also:
- install the ` + "`p0_azure_bastion_host_staged`" + ` resource,
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,
- install the ` + "`p0_azure_iam_write`" + ` resource for the same subscription,
- create an Azure Bastion host (e.g. via the ` + "`azure_p0_bastion`" + ` module),
- create and assign the P0 Bastion Host Management role to the P0 app (e.g. via the ` + "`azure_p0_roles`" + ` module).

Use ` + "`p0_azure_bastion_host_staged`" + ` computed ` + "`custom_role`" + ` when defining that Azure role. See ` + "`examples/resources/p0_azure_bastion_host/`" + ` for a full chain.

` + "\n\nExample (after creating the Bastion and role in Azure):\n\n```terraform\n" +
			"resource \"p0_azure_bastion_host\" \"example\" {\n" +
			"  subscription_id    = p0_azure_bastion_host_staged.example.subscription_id\n" +
			"  bastion_id         = module.azure_p0_bastion.bastion_resource_id\n" +
			"  role_definition_id = module.azure_p0_roles.bastion_role_definition_id\n" +
			"}\n" +
			"```\n",
		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				Description: "The Azure subscription ID where the bastion host is used.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bastion_id": schema.StringAttribute{
				Description: "The full Azure resource ID of the Bastion host (e.g. from azure_p0_bastion.bastion_resource_id).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role_definition_id": schema.StringAttribute{
				Description: "The Azure role definition ID for the P0 Bastion Host Management role (e.g. from azure_p0_roles.bastion_role_definition_id).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				Description: "The label of the subscription (computed from P0).",
				Computed:    true,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *azureBastionHost) getId(data any) *string {
	model, ok := data.(*azureBastionHostModel)
	if !ok {
		return nil
	}
	return model.SubscriptionId.ValueStringPointer()
}

func (r *azureBastionHost) getItemJson(json any) any {
	inner, ok := json.(*bastionHostApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *azureBastionHost) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureBastionHostModel{}
	jsonv, ok := json.(*bastionHostItemJson)
	if !ok {
		return nil
	}

	data.SubscriptionId = types.StringValue(id)
	data.State = types.StringValue(jsonv.State)
	data.Label = types.StringValue(jsonv.Label)
	data.RoleDefinitionId = types.StringValue(jsonv.RoleDefinitionId)
	if jsonv.Bastion.Type == "single" {
		data.BastionId = jsonv.Bastion.BastionId
	}

	return &data
}

func (r *azureBastionHost) toJson(data any) any {
	datav, ok := data.(*azureBastionHostModel)
	if !ok {
		return nil
	}
	if datav.RoleDefinitionId.IsUnknown() || datav.RoleDefinitionId.IsNull() {
		return nil
	}
	return &bastionHostItemJson{
		Bastion: bastionHostBastionRef{
			Type:      "single",
			BastionId: datav.BastionId,
		},
		RoleDefinitionId: datav.RoleDefinitionId.ValueString(),
	}
}

func (r *azureBastionHost) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (s *azureBastionHost) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data azureBastionHostModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := s.installer.ToJson(&data)
	if inputJson == nil {
		resp.Diagnostics.AddError("Bad Terraform state", "Could not represent bastion host as JSON")
		return
	}

	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &bastionHostApi{}, &data, inputJson)
	if resp.Diagnostics.HasError() {
		return
	}

	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &bastionHostApi{}, &azureBastionHostModel{})
}

func (s *azureBastionHost) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &bastionHostApi{}, &azureBastionHostModel{})
}

func (s *azureBastionHost) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &bastionHostApi{}, &azureBastionHostModel{})
}

func (s *azureBastionHost) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &azureBastionHostModel{})
}

func (s *azureBastionHost) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("subscription_id"), req, resp)
}
