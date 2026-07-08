// The bastion-host component registers how P0 connects to Azure VMs in a subscription to
// provision SSH access. Exactly one connection method is configured per subscription:
//
//   - `azure_bastion`: a managed Azure Bastion host, plus the P0 Bastion Host Management
//     custom role. Stage with `p0_azure_bastion_host_staged`, create the role and Bastion in
//     Azure (for example with the `azure_p0_roles` and `azure_p0_bastion` modules), then pass
//     the Bastion ARM ID and role definition ID here.
//   - `jump_host`: a customer-managed jump host VM, referenced by its resource ID. No custom
//     role or staged resource is needed; P0 resolves and stores the VM's public IP at install
//     time.
//
// See `examples/resources/p0_azure_bastion_host/`.

package installazure

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Discriminators of the API's `bastion` union (shared/src/integrations/resources/azure/components.ts).
const (
	azureBastionType = "azureBastion"
	jumpHostType     = "jumpHost"
	// The only bastion-host reference this resource creates; "subscription"
	// (borrow another subscription's Bastion) is configurable in the P0 app only.
	singleBastionHostType = "single"
)

// Mirror the backend's resource-ID validation (shared/src/integrations/resources/azure/asset.ts).
// Azure resource IDs compare case-insensitively. Role definition IDs are accepted at any scope
// from which they can be referenced: subscription-scoped, management-group-scoped, or unscoped
// (built-in roles, and custom roles referenced from their defining scope's root).
var roleDefinitionResourceIdRegex = regexp.MustCompile(
	`(?i)^(?:/subscriptions/[^/]+|/providers/Microsoft\.Management/managementGroups/[^/]+)?/providers/Microsoft\.Authorization/roleDefinitions/[^/]+$`,
)

var virtualMachineResourceIdRegex = regexp.MustCompile(
	`(?i)^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft\.Compute/virtualMachines/[^/]+$`,
)

var bastionHostResourceIdRegex = regexp.MustCompile(
	`(?i)^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft\.Network/bastionHosts/[^/]+$`,
)

const (
	roleDefinitionResourceIdExample = "/subscriptions/<id>/providers/Microsoft.Authorization/roleDefinitions/<guid>"
	virtualMachineResourceIdExample = "/subscriptions/<id>/resourceGroups/<rg>/providers/Microsoft.Compute/virtualMachines/<name>"
	bastionHostResourceIdExample    = "/subscriptions/<id>/resourceGroups/<rg>/providers/Microsoft.Network/bastionHosts/<name>"
)

var _ resource.Resource = &azureBastionHost{}
var _ resource.ResourceWithImportState = &azureBastionHost{}
var _ resource.ResourceWithConfigure = &azureBastionHost{}
var _ resource.ResourceWithConfigValidators = &azureBastionHost{}
var _ resource.ResourceWithUpgradeState = &azureBastionHost{}

func NewAzureBastionHost() resource.Resource {
	return &azureBastionHost{}
}

type azureBastionHost struct {
	installer *common.Install
}

type azureBastionHostAzureBastionModel struct {
	BastionId        string `tfsdk:"bastion_id"`
	RoleDefinitionId string `tfsdk:"role_definition_id"`
}

type azureBastionHostJumpHostModel struct {
	VirtualMachineId string       `tfsdk:"virtual_machine_id"`
	RoleDefinitionId string       `tfsdk:"role_definition_id"`
	Ip               types.String `tfsdk:"ip"`
}

type azureBastionHostModel struct {
	SubscriptionId types.String                       `tfsdk:"subscription_id"`
	AzureBastion   *azureBastionHostAzureBastionModel `tfsdk:"azure_bastion"`
	JumpHost       *azureBastionHostJumpHostModel     `tfsdk:"jump_host"`
	Label          types.String                       `tfsdk:"label"`
	State          types.String                       `tfsdk:"state"`
}

// Item request/response for the P0 API (camelCase for API). `bastion` is a
// discriminated union: `azureBastion` fields and `jumpHost` fields are
// mutually exclusive.
type bastionHostBastionHostRefJson struct {
	Type      string `json:"type"`
	BastionId string `json:"bastionId,omitempty"`
}

type bastionHostBastionJson struct {
	Type string `json:"type"`
	// Both options: the P0 Bastion Host Management custom role for azureBastion,
	// or the customer-owned role granted to connecting users for jumpHost.
	RoleDefinitionId string `json:"roleDefinitionId,omitempty"`
	// azureBastion fields
	BastionHost *bastionHostBastionHostRefJson `json:"bastionHost,omitempty"`
	// jumpHost fields; `ip` is resolved server-side from the VM at install time
	VirtualMachineId string `json:"virtualMachineId,omitempty"`
	Ip               string `json:"ip,omitempty"`
}

type bastionHostItemJson struct {
	Bastion bastionHostBastionJson `json:"bastion"`
	State   string                 `json:"state,omitempty"`
	Label   string                 `json:"label,omitempty"`
}

type bastionHostApi struct {
	Item bastionHostItemJson `json:"item"`
}

func (r *azureBastionHost) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_bastion_host"
}

func (r *azureBastionHost) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Version 1: flat `bastion_id`/`role_definition_id` moved into the `azure_bastion`
		// nested attribute (see UpgradeState).
		Version: 1,
		MarkdownDescription: `Registers how P0 connects to Azure VMs in a subscription to provision SSH access: through a managed Azure Bastion host, or through a customer-managed jump host VM. Configure exactly one of ` + "`azure_bastion`" + ` or ` + "`jump_host`" + `.

In both cases, you must also:
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,
- install the ` + "`p0_azure_iam_write`" + ` resource for the same subscription.

To use ` + "`azure_bastion`" + `, you must additionally:
- install the ` + "`p0_azure_bastion_host_staged`" + ` resource,
- create an Azure Bastion host (e.g. via the ` + "`azure_p0_bastion`" + ` module),
- create and assign the P0 Bastion Host Management role to the P0 app (e.g. via the ` + "`azure_p0_roles`" + ` module), using the staged resource's computed ` + "`custom_role`" + `.

To use ` + "`jump_host`" + `, the VM must have a public IP address on its primary network interface; P0 resolves and stores the IP at install time. You must also provide the role definition ID of the role granted to users connecting through the jump host (a built-in role, an existing custom role, or a new one with at least the required permissions — see the P0 setup instructions). No staged resource or Bastion host is needed. To let P0 terminate established jump host sessions when access is revoked, also install the ` + "`p0_azure_jump_host`" + ` management component.

See ` + "`examples/resources/p0_azure_bastion_host/`" + ` for full chains.

` + "\n\nExample (after creating the Bastion and role in Azure):\n\n```terraform\n" +
			"resource \"p0_azure_bastion_host\" \"example\" {\n" +
			"  subscription_id = p0_azure_bastion_host_staged.example.subscription_id\n" +
			"  azure_bastion = {\n" +
			"    bastion_id         = module.azure_p0_bastion.bastion_resource_id\n" +
			"    role_definition_id = module.azure_p0_roles.bastion_role_definition_id\n" +
			"  }\n" +
			"}\n" +
			"```\n" +
			"\nOr, with a customer-managed jump host VM:\n\n```terraform\n" +
			"resource \"p0_azure_bastion_host\" \"example\" {\n" +
			"  subscription_id = local.subscription_id\n" +
			"  jump_host = {\n" +
			"    virtual_machine_id = \"" + virtualMachineResourceIdExample + "\"\n" +
			"    role_definition_id = \"" + roleDefinitionResourceIdExample + "\"\n" +
			"  }\n" +
			"}\n" +
			"```\n",
		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				Description: "The Azure subscription ID where the bastion host or jump host is used.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"azure_bastion": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Provision SSH access through a managed Azure Bastion host. Exactly one of `azure_bastion` or `jump_host` must be configured.",
				Attributes: map[string]schema.Attribute{
					"bastion_id": schema.StringAttribute{
						Description: "The full Azure resource ID of the Bastion host (e.g. from azure_p0_bastion.bastion_resource_id).",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(bastionHostResourceIdRegex, "Enter a valid Bastion host resource ID, e.g. "+bastionHostResourceIdExample+"."),
						},
					},
					"role_definition_id": schema.StringAttribute{
						Description: "The Azure role definition ID for the P0 Bastion Host Management role (e.g. from azure_p0_roles.bastion_role_definition_id).",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(roleDefinitionResourceIdRegex, "Enter a valid role definition resource ID, e.g. "+roleDefinitionResourceIdExample+"."),
						},
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"jump_host": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Provision SSH access through a customer-managed jump host VM. Exactly one of `azure_bastion` or `jump_host` must be configured.",
				Attributes: map[string]schema.Attribute{
					"virtual_machine_id": schema.StringAttribute{
						Description: "The Azure resource ID of the jump host VM, e.g. " + virtualMachineResourceIdExample + ".",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(virtualMachineResourceIdRegex, "Enter a valid virtual machine resource ID, e.g. "+virtualMachineResourceIdExample+"."),
						},
					},
					"role_definition_id": schema.StringAttribute{
						Description: "The Azure role definition ID of the role granted to users connecting through this jump host, so they can reach a target virtual machine. Use a built-in role, an existing custom role, or create a new one with at least the required permissions.",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(roleDefinitionResourceIdRegex, "Enter a valid role definition resource ID, e.g. "+roleDefinitionResourceIdExample+"."),
						},
					},
					"ip": schema.StringAttribute{
						Description: "The jump host's public IP address, resolved from the VM's primary network interface by P0 at install time (computed from P0).",
						Computed:    true,
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				Description: "The label of this install: the subscription label for azure_bastion, or the VM name for jump_host (computed from P0).",
				Computed:    true,
			},
			"state": common.StateAttribute,
		},
	}
}

// Schema version 0 (provider <= v0.44.0) had flat `bastion_id` and `role_definition_id`
// attributes; only the Azure Bastion connection method existed then. They map onto the
// `azure_bastion` nested attribute.
type azureBastionHostModelV0 struct {
	SubscriptionId   types.String `tfsdk:"subscription_id"`
	BastionId        string       `tfsdk:"bastion_id"`
	RoleDefinitionId types.String `tfsdk:"role_definition_id"`
	Label            types.String `tfsdk:"label"`
	State            types.String `tfsdk:"state"`
}

func (r *azureBastionHost) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"subscription_id":    schema.StringAttribute{Required: true},
					"bastion_id":         schema.StringAttribute{Required: true},
					"role_definition_id": schema.StringAttribute{Required: true},
					"label":              schema.StringAttribute{Computed: true},
					"state":              common.StateAttribute,
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior azureBastionHostModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgraded := azureBastionHostModel{
					SubscriptionId: prior.SubscriptionId,
					AzureBastion: &azureBastionHostAzureBastionModel{
						BastionId:        prior.BastionId,
						RoleDefinitionId: prior.RoleDefinitionId.ValueString(),
					},
					Label: prior.Label,
					State: prior.State,
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, &upgraded)...)
			},
		},
	}
}

func (r *azureBastionHost) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("azure_bastion"),
			path.MatchRoot("jump_host"),
		),
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

	switch jsonv.Bastion.Type {
	case azureBastionType:
		// This resource can only manage a directly-referenced ("single") bastion
		// host; a "subscription" reference (borrowing another subscription's
		// bastion) is configurable in the P0 app only. Erroring beats writing
		// partial state: with these attributes marked RequiresReplace, partial
		// state would produce a replacement plan that clobbers the app-side
		// configuration on apply.
		if jsonv.Bastion.BastionHost == nil || jsonv.Bastion.BastionHost.Type != singleBastionHostType {
			refType := "<missing>"
			if jsonv.Bastion.BastionHost != nil {
				refType = jsonv.Bastion.BastionHost.Type
			}
			diags.AddError(
				"Unsupported bastion host reference",
				fmt.Sprintf("The bastion-host install for subscription %s references a bastion host of type %q; this resource can only manage type %q. "+
					"It was likely reconfigured in the P0 app — manage it there, or remove it from Terraform state with `terraform state rm`.",
					id, refType, singleBastionHostType),
			)
			return nil
		}
		data.AzureBastion = &azureBastionHostAzureBastionModel{
			BastionId:        jsonv.Bastion.BastionHost.BastionId,
			RoleDefinitionId: jsonv.Bastion.RoleDefinitionId,
		}
	case jumpHostType:
		data.JumpHost = &azureBastionHostJumpHostModel{
			VirtualMachineId: jsonv.Bastion.VirtualMachineId,
			RoleDefinitionId: jsonv.Bastion.RoleDefinitionId,
			Ip:               types.StringValue(jsonv.Bastion.Ip),
		}
	default:
		diags.AddError(
			"Unsupported bastion configuration",
			fmt.Sprintf("The bastion-host install for subscription %s has bastion type %q, which this resource cannot manage. "+
				"The install may have been created or reconfigured outside Terraform, or may require a newer provider version. "+
				"Manage it in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Bastion.Type),
		)
		return nil
	}

	return &data
}

func (r *azureBastionHost) toJson(data any) any {
	datav, ok := data.(*azureBastionHostModel)
	if !ok {
		return nil
	}

	if datav.AzureBastion != nil {
		return &bastionHostItemJson{
			Bastion: bastionHostBastionJson{
				Type:             azureBastionType,
				RoleDefinitionId: datav.AzureBastion.RoleDefinitionId,
				BastionHost: &bastionHostBastionHostRefJson{
					Type:      singleBastionHostType,
					BastionId: datav.AzureBastion.BastionId,
				},
			},
		}
	}

	if datav.JumpHost != nil {
		// `ip` is omitted; the backend resolves it from the VM at install time.
		return &bastionHostItemJson{
			Bastion: bastionHostBastionJson{
				Type:             jumpHostType,
				VirtualMachineId: datav.JumpHost.VirtualMachineId,
				RoleDefinitionId: datav.JumpHost.RoleDefinitionId,
			},
		}
	}

	return nil
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
