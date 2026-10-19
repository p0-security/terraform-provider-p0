package installentraagentidentity

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

var _ resource.Resource = &EntraAgentIdentityIamWriteStaged{}
var _ resource.ResourceWithConfigure = &EntraAgentIdentityIamWriteStaged{}
var _ resource.ResourceWithImportState = &EntraAgentIdentityIamWriteStaged{}

type EntraAgentIdentityIamWriteStaged struct {
	installer *common.Install
}

type entraAgentIdentityIamWriteStagedModel struct {
	DirectoryId         types.String `tfsdk:"directory_id"`
	SubscriptionId      types.String `tfsdk:"subscription_id"`
	Region              types.String `tfsdk:"region"`
	AppRoleAllowlist    types.String `tfsdk:"app_role_allowlist"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	State               types.String `tfsdk:"state"`
}

type entraAgentIdentityIamWriteStagedJson struct {
	SubscriptionId      *string `json:"subscriptionId,omitempty"`
	Region              *string `json:"region,omitempty"`
	AppRoleAllowlist    *string `json:"appRoleAllowlist,omitempty"`
	ServiceAccountEmail *string `json:"serviceAccountEmail,omitempty"`
	ServiceAccountId    *string `json:"serviceAccountId,omitempty"`
	State               *string `json:"state,omitempty"`
}

type entraAgentIdentityIamWriteStagedApi struct {
	Item *entraAgentIdentityIamWriteStagedJson `json:"item"`
}

func NewEntraAgentIdentityIamWriteStaged() resource.Resource {
	return &EntraAgentIdentityIamWriteStaged{}
}

func (*EntraAgentIdentityIamWriteStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_entra_agent_identity_creator_staged"
}

// P0 accepts these only when it creates the item, since the connector's Terraform is
// rendered from them, so a change replaces the install.
func stagedInputAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func (*EntraAgentIdentityIamWriteStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Entra Agent Identity Creator installation for a Microsoft Entra directory. Staging picks the P0 service account that calls the connector, which the connector's app registration must trust.

Use the read-only ` + "`service_account_id`" + ` attribute as the subject of that app registration's federated identity credential, and deploy the connector's Container App with the same ` + "`app_role_allowlist`" + `. Once the connector is deployed, create a ` + "`p0_entra_agent_identity_creator`" + ` resource with the same ` + "`directory_id`" + ` to complete the installation.

**Prerequisites:** An Azure subscription that trusts the directory, and a P0 gateway, whose issuer the connector trusts.

A directory connects to one P0 organization, and P0 refuses to stage a directory that another P0 organization already uses. Each directory gets its own P0 service account, so for another service account, install the Entra Agent Identity Creator for another directory.

` + previewNote + `

` + docsLink,
		Attributes: map[string]schema.Attribute{
			"directory_id": directoryIdAttribute(`The Microsoft Entra directory (tenant) ID, in lowercase, that P0 creates an app registration in for each agentic grant`),
			"subscription_id": stagedInputAttribute(
				`The Azure subscription ID, in lowercase, that the connector's Container App is deployed in`,
				guidValidator("subscription ID"),
			),
			"region": stagedInputAttribute(
				`The Azure region the connector runs in, e.g. `+"`eastus`",
				stringvalidator.RegexMatches(azureRegionRegex, "Enter an Azure region name, e.g. eastus"),
			),
			"app_role_allowlist": stagedInputAttribute(
				`The Microsoft Graph app roles a per-grant app may hold, separated by commas, e.g. `+"`Sites.Selected,User.Read.All`"+`. The connector refuses any other. Set the connector's `+"`P0_APP_ROLE_ALLOWLIST`"+` environment variable to the same value.`,
				stringvalidator.RegexMatches(appRoleAllowlistRegex, "Enter Microsoft Graph app role values separated by commas, e.g. Sites.Selected,User.Read.All"),
			),
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the P0 service account that calls the connector`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The numeric ID of the P0 service account that calls the connector. The connector of every Microsoft 365 integration in the directory trusts this account: the connector's app registration names it as the subject of a federated identity credential with issuer ` + "`https://accounts.google.com`" + `.`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *EntraAgentIdentityIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:           EntraAgentIdentityKey,
		Component:             installresources.IamWrite,
		ProviderData:          data,
		GetId:                 r.getId,
		GetItemJson:           r.getItemJson,
		FromJson:              r.fromJson,
		ToJson:                r.toJson,
		DescribeWriteNotFound: describeNotOffered,
	}
}

// The item is keyed by the directory.
func (r *EntraAgentIdentityIamWriteStaged) getId(data any) *string {
	model, ok := data.(*entraAgentIdentityIamWriteStagedModel)
	if !ok {
		return nil
	}
	str := model.DirectoryId.ValueString()
	return &str
}

func (r *EntraAgentIdentityIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*entraAgentIdentityIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *EntraAgentIdentityIamWriteStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*entraAgentIdentityIamWriteStagedJson)
	if !ok {
		return nil
	}

	return &entraAgentIdentityIamWriteStagedModel{
		DirectoryId:         types.StringValue(id),
		SubscriptionId:      types.StringPointerValue(jsonv.SubscriptionId),
		Region:              types.StringPointerValue(jsonv.Region),
		AppRoleAllowlist:    types.StringPointerValue(jsonv.AppRoleAllowlist),
		ServiceAccountEmail: types.StringPointerValue(jsonv.ServiceAccountEmail),
		ServiceAccountId:    types.StringPointerValue(jsonv.ServiceAccountId),
		State:               types.StringPointerValue(jsonv.State),
	}
}

// Sends only the inputs: P0 generates the service account fields.
func (r *EntraAgentIdentityIamWriteStaged) toJson(data any) any {
	datav, ok := data.(*entraAgentIdentityIamWriteStagedModel)
	if !ok {
		return nil
	}
	return &entraAgentIdentityIamWriteStagedJson{
		SubscriptionId:   datav.SubscriptionId.ValueStringPointer(),
		Region:           datav.Region.ValueStringPointer(),
		AppRoleAllowlist: datav.AppRoleAllowlist.ValueStringPointer(),
	}
}

func (r *EntraAgentIdentityIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraAgentIdentityIamWriteStagedApi
	var data entraAgentIdentityIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *EntraAgentIdentityIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraAgentIdentityIamWriteStagedApi{}, &entraAgentIdentityIamWriteStagedModel{})
}

func (r *EntraAgentIdentityIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json entraAgentIdentityIamWriteStagedApi
	var data entraAgentIdentityIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *EntraAgentIdentityIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &entraAgentIdentityIamWriteStagedModel{})
}

func (r *EntraAgentIdentityIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("directory_id"), req, resp)
}
