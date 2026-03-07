package entra_id

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const entraIdConfigPath = "integrations/entra-id/config"

type entraIdConfigResponse struct {
	Config struct {
		Base     map[string]entraBaseItem `json:"base"`
		IamWrite map[string]entraIamWriteItem `json:"iam-write"`
	} `json:"config"`
}

type entraBaseItem struct {
	ServiceAccountId    string `json:"serviceAccountId"`
	ServiceAccountEmail string `json:"serviceAccountEmail"`
	State               string `json:"state"`
}

type entraIamWriteItem struct {
	State string `json:"state"`
}

var _ resource.Resource = &EntraIdIamWriteStaged{}
var _ resource.ResourceWithImportState = &EntraIdIamWriteStaged{}
var _ resource.ResourceWithConfigure = &EntraIdIamWriteStaged{}

func NewEntraIdIamWriteStaged() resource.Resource {
	return &EntraIdIamWriteStaged{}
}

type EntraIdIamWriteStaged struct {
	providerData *internal.P0ProviderData
}

type EntraIdIamWriteStagedModel struct {
	TenantId            string       `tfsdk:"tenant_id"`
	State               types.String `tfsdk:"state"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
}

func (r *EntraIdIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_entra_id_iam_write_staged"
}

func (r *EntraIdIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Stages Entra ID (Azure AD) IAM write for a tenant. Create the caller app and Security Perimeter, then use `p0_entra_id_iam_write` to complete installation.",
		Attributes: map[string]schema.Attribute{
			"tenant_id":             tenantIdAttribute,
			"state":                 stateAttribute,
			"service_account_id": schema.StringAttribute{
				Description: "P0 service account ID assigned for this tenant (from P0 after stage).",
				Computed:    true,
			},
			"service_account_email": schema.StringAttribute{
				Description: "P0 service account email assigned for this tenant (from P0 after stage).",
				Computed:    true,
			},
		},
	}
}

func (r *EntraIdIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.providerData = internal.Configure(&req, resp)
}

func (r *EntraIdIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Provider not configured", "ProviderData is nil; configure the provider before creating the resource.")
		return
	}
	var plan EntraIdIamWriteStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tenantId := plan.TenantId
	if tenantId == "" {
		resp.Diagnostics.AddError("Missing tenant_id", "tenant_id is required")
		return
	}

	var discard struct{}
	_, err := r.providerData.Post(entraIdConfigPath, struct{}{}, &discard)
	if err != nil && !strings.Contains(err.Error(), "409 Conflict") {
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Failed to ensure Entra ID config, got error: %s", err))
		return
	}

	itemPath := fmt.Sprintf("%s/%s/%s", entraIdConfigPath, installresources.IamWrite, tenantId)
	_, err = r.providerData.Put(itemPath, struct{}{}, &discard)
	if err != nil {
		resp.Diagnostics.AddError("Could not stage Entra ID iam-write", fmt.Sprintf("Error: %s", err))
		return
	}

	var configResp entraIdConfigResponse
	_, err = r.providerData.Get(entraIdConfigPath, &configResp)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Entra ID config", fmt.Sprintf("Error: %s", err))
		return
	}

	if configResp.Config.Base == nil || configResp.Config.IamWrite == nil {
		resp.Diagnostics.AddError("Bad API response", "Config missing base or iam-write after stage")
		return
	}
	baseItem, hasBase := configResp.Config.Base[tenantId]
	iamItem, hasIam := configResp.Config.IamWrite[tenantId]
	if !hasBase || !hasIam {
		resp.Diagnostics.AddError("Bad API response", "Config missing base or iam-write item after stage")
		return
	}

	plan.ServiceAccountId = types.StringValue(baseItem.ServiceAccountId)
	plan.ServiceAccountEmail = types.StringValue(baseItem.ServiceAccountEmail)
	plan.State = types.StringValue(iamItem.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *EntraIdIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.providerData == nil {
		return
	}
	var state EntraIdIamWriteStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tenantId := state.TenantId
	if tenantId == "" {
		resp.Diagnostics.AddError("Invalid state", "tenant_id is empty in state")
		return
	}

	var configResp entraIdConfigResponse
	httpResp, httpErr := r.providerData.Get(entraIdConfigPath, &configResp)
	if httpResp != nil && httpResp.StatusCode == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if httpErr != nil {
		resp.Diagnostics.AddError("Error reading Entra ID config", fmt.Sprintf("Error: %s", httpErr))
		return
	}

	if configResp.Config.Base == nil || configResp.Config.IamWrite == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	baseItem, hasBase := configResp.Config.Base[tenantId]
	iamItem, hasIam := configResp.Config.IamWrite[tenantId]
	if !hasBase || !hasIam {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ServiceAccountId = types.StringValue(baseItem.ServiceAccountId)
	state.ServiceAccountEmail = types.StringValue(baseItem.ServiceAccountEmail)
	state.State = types.StringValue(iamItem.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *EntraIdIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan EntraIdIamWriteStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *EntraIdIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.providerData == nil {
		return
	}
	var state EntraIdIamWriteStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.TenantId == "" {
		return
	}
	itemPath := fmt.Sprintf("%s/%s/%s", entraIdConfigPath, installresources.IamWrite, state.TenantId)
	httpResp, err := r.providerData.Delete(itemPath)
	if httpResp != nil && httpResp.StatusCode == 404 {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting Entra ID iam-write", fmt.Sprintf("Error: %s", err))
	}
}

func (r *EntraIdIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}
