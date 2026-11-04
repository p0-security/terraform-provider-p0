package installentraid

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &EntraIdStaged{}
var _ resource.ResourceWithConfigure = &EntraIdStaged{}
var _ resource.ResourceWithImportState = &EntraIdStaged{}

type EntraIdStaged struct {
	installer *common.Install
}

type entraIdStagedModel struct {
	TenantId            types.String `tfsdk:"tenant_id"`
	SovereignCloudId    types.String `tfsdk:"sovereign_cloud_id"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	State               types.String `tfsdk:"state"`
}

type entraIdStagedJson struct {
	SovereignCloudId *string `json:"sovereignCloudId,omitempty"`
	State            *string `json:"state,omitempty"`
}

type entraIdStagedApi struct {
	Item *entraIdStagedJson `json:"item"`
}

type entraIdBaseApi struct {
	Item *struct {
		ServiceAccountId    *string `json:"serviceAccountId"`
		ServiceAccountEmail *string `json:"serviceAccountEmail"`
	} `json:"item"`
}

func NewEntraIdStaged() resource.Resource {
	return &EntraIdStaged{}
}

func (*EntraIdStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id_staged"
}

func (*EntraIdStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Microsoft Entra ID installation, for just-in-time access to Entra directory roles and groups.

Staging assigns the service account that P0 signs in to the tenant as. Create P0's caller app registration in the tenant, with a federated identity credential whose subject is ` + "`service_account_id`" + `, issuer ` + "`https://accounts.google.com`" + ` and audience ` + "`api://AzureADTokenExchange`" + `, and grant it its Microsoft Graph application permissions. Then complete the install with ` + "`p0_entra_id`" + ` and the app's client ID.

For the full install steps, see [Microsoft Entra ID](https://docs.p0.dev/integrations/directory-integrations/microsoft-entra-id).`,
		Attributes: map[string]schema.Attribute{
			"tenant_id":          tenantIdAttribute(""),
			"sovereign_cloud_id": sovereignCloudIdAttribute(""),
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the Google service account that P0 signs in to the tenant as. Use it as the subject of the caller app's federated identity credential.",
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of the Google service account that P0 signs in to the tenant as.",
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *EntraIdStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  EntraIdKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *EntraIdStaged) getId(data any) *string {
	model, ok := data.(*entraIdStagedModel)
	if !ok {
		return nil
	}
	str := model.TenantId.ValueString()
	return &str
}

func (r *EntraIdStaged) getItemJson(json any) any {
	inner, ok := json.(*entraIdStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

// The service account fields come from the base item; readBase fills them.
func (r *EntraIdStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*entraIdStagedJson)
	if !ok {
		return nil
	}
	return &entraIdStagedModel{
		TenantId:            types.StringValue(id),
		SovereignCloudId:    sovereignCloudIdFromJson(jsonv.SovereignCloudId),
		ServiceAccountId:    types.StringNull(),
		ServiceAccountEmail: types.StringNull(),
		State:               types.StringPointerValue(jsonv.State),
	}
}

func (r *EntraIdStaged) toJson(data any) any {
	datav, ok := data.(*entraIdStagedModel)
	if !ok {
		return nil
	}
	return &entraIdStagedJson{SovereignCloudId: datav.SovereignCloudId.ValueStringPointer()}
}

// Sets the service account fields from the base item that P0 made when it staged the
// tenant.
func (r *EntraIdStaged) readBase(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State) {
	var tenantId types.String
	diags.Append(state.GetAttribute(ctx, path.Root("tenant_id"), &tenantId)...)
	if diags.HasError() {
		return
	}

	var base entraIdBaseApi
	resp, err := r.installer.ProviderData.Get(basePath(tenantId.ValueString()), &base)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		diags.AddError("Entra ID service account missing",
			fmt.Sprintf("P0 has no service account for the Entra tenant %q. Destroy p0_entra_id_staged and apply again to stage the tenant again.", tenantId.ValueString()))
		return
	}
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to read the Entra ID service account, got error:\n%s", err))
		return
	}
	if base.Item == nil || base.Item.ServiceAccountId == nil {
		diags.AddError("Bad API response", fmt.Sprintf("P0 returned no service account for the Entra tenant %q.", tenantId.ValueString()))
		return
	}

	diags.Append(state.SetAttribute(ctx, path.Root("service_account_id"), types.StringPointerValue(base.Item.ServiceAccountId))...)
	diags.Append(state.SetAttribute(ctx, path.Root("service_account_email"), types.StringPointerValue(base.Item.ServiceAccountEmail))...)
}

func (r *EntraIdStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json entraIdStagedApi
	var data entraIdStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readBase(ctx, &resp.Diagnostics, &resp.State)
}

func (r *EntraIdStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &entraIdStagedApi{}, &entraIdStagedModel{})
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		return
	}

	r.readBase(ctx, &resp.Diagnostics, &resp.State)
}

// Every configurable attribute replaces the resource.
func (r *EntraIdStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cannot Update", "Modifying P0's staged Entra ID install forces replacement")
}

func (r *EntraIdStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data entraIdStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.installer.ProviderData.Delete(basePath(data.TenantId.ValueString()))
	if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Could not delete the Entra ID service account, got error: %s", err))
	}
}

func (r *EntraIdStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
