package installentraid

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &EntraId{}
var _ resource.ResourceWithConfigure = &EntraId{}
var _ resource.ResourceWithImportState = &EntraId{}

type EntraId struct {
	installer *common.Install
}

type entraIdModel struct {
	TenantId         types.String `tfsdk:"tenant_id"`
	SovereignCloudId types.String `tfsdk:"sovereign_cloud_id"`
	ClientId         types.String `tfsdk:"client_id"`
	EmailField       types.String `tfsdk:"email_field"`
	Label            types.String `tfsdk:"label"`
	State            types.String `tfsdk:"state"`
}

type entraIdJson struct {
	SovereignCloudId *optionJson `json:"sovereignCloudId,omitempty"`
	ClientId         *string     `json:"clientId,omitempty"`
	EmailField       *optionJson `json:"emailField,omitempty"`
	Label            *string     `json:"label,omitempty"`
	State            *string     `json:"state,omitempty"`
}

type entraIdApi struct {
	Item *entraIdJson `json:"item"`
}

func NewEntraId() resource.Resource {
	return &EntraId{}
}

func (*EntraId) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id"
}

// Set it from the staged resource's attribute, so a new tenant or cloud replaces
// both resources.
const sameAsStaged = " Must match the `p0_entra_id_staged` resource."

func (*EntraId) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Microsoft Entra ID installation, for just-in-time access to Entra directory roles and groups.

To use this resource, you must first:
- install the ` + "`p0_entra_id_staged`" + ` resource,
- create P0's caller app registration in the tenant, with its federated identity credential and its Microsoft Graph application permissions, granted with admin consent.

On install, P0 signs in to the tenant as the caller app and lists a group and a role. Role and group assignments go through the P0 Security Perimeter, which you deploy in Azure separately.

For the full install steps, see [Microsoft Entra ID](https://docs.p0.dev/integrations/directory-integrations/microsoft-entra-id).`,
		Attributes: map[string]schema.Attribute{
			"tenant_id":          tenantIdAttribute(sameAsStaged),
			"sovereign_cloud_id": sovereignCloudIdAttribute(sameAsStaged),
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The client (application) ID of P0's caller app registration in the tenant.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Entra application client ID must be a valid UUID"),
				},
			},
			"email_field": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(emailFields[0]),
				MarkdownDescription: "The user property that holds each user's email: `userPrincipalName` (the default), `mail` or `otherMails`.",
				Validators: []validator.String{
					stringvalidator.OneOf(emailFields...),
				},
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The tenant's display name in Entra ID.",
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *EntraId) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        EntraIdKey,
		Component:          installresources.IamWrite,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError,
	}
}

func (r *EntraId) getId(data any) *string {
	model, ok := data.(*entraIdModel)
	if !ok {
		return nil
	}
	str := model.TenantId.ValueString()
	return &str
}

func (r *EntraId) getItemJson(json any) any {
	inner, ok := json.(*entraIdApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *EntraId) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*entraIdJson)
	if !ok {
		return nil
	}
	return &entraIdModel{
		TenantId:         types.StringValue(id),
		SovereignCloudId: optionFromJson(jsonv.SovereignCloudId, sovereignClouds[0]),
		ClientId:         types.StringPointerValue(jsonv.ClientId),
		EmailField:       optionFromJson(jsonv.EmailField, emailFields[0]),
		Label:            types.StringPointerValue(jsonv.Label),
		State:            types.StringPointerValue(jsonv.State),
	}
}

func (r *EntraId) toJson(data any) any {
	datav, ok := data.(*entraIdModel)
	if !ok {
		return nil
	}
	// The cloud is fixed at staging, so it isn't sent.
	return &entraIdJson{
		ClientId:   datav.ClientId.ValueStringPointer(),
		EmailField: optionToJson(datav.EmailField),
	}
}

func (r *EntraId) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdApi{}, &entraIdModel{})
}

func (r *EntraId) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraIdApi{}, &entraIdModel{})
}

// P0 checks the install on configure alone, so an item that P0 has verified gets the
// configure step alone, and a failed check leaves the item and state as they were. A
// staged item is verified first, as Create does.
func (r *EntraId) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdApi{}, &entraIdModel{})
	default:
		r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &entraIdApi{}, &entraIdModel{})
	}
}

// Returns the item to staged, so p0_entra_id_staged still owns it.
func (r *EntraId) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &entraIdModel{})
}

func (r *EntraId) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
