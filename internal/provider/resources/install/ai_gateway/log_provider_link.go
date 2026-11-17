// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaigateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &LogProviderLink{}
var _ resource.ResourceWithImportState = &LogProviderLink{}
var _ resource.ResourceWithConfigure = &LogProviderLink{}

func NewLogProviderLink() resource.Resource {
	return &LogProviderLink{}
}

// LogProviderLink owns one field of a gateway item: its `log-provider`. It
// writes that field alone, so p0_ai_gateway, which never sends it, can be
// applied again without clearing the link.
type LogProviderLink struct {
	installer *common.Install
}

type logProviderLinkModel struct {
	Id          types.String `tfsdk:"id"`
	Gateway     types.String `tfsdk:"gateway"`
	LogProvider types.String `tfsdk:"log_provider"`
}

// logProviderLinkJson reads only the gateway item's `log-provider`. The item
// holds "" once a gateway is unlinked.
type logProviderLinkJson struct {
	LogProvider *string `json:"log-provider,omitempty"`
}

type logProviderLinkApi struct {
	Item logProviderLinkJson `json:"item"`
}

// logProviderLinkConfigureJson is the whole request body. P0 merges it over the
// stored gateway item, so every other gateway field keeps its value. Its state
// matches gatewayConfigureJson's.
type logProviderLinkConfigureJson struct {
	LogProvider string `json:"log-provider"`
	State       string `json:"state"`
}

func (r *LogProviderLink) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_gateway_log_provider_link"
}

func (r *LogProviderLink) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Links an installed AI gateway to a log provider, so P0 can show the gateway's MCP tool-call
activity in the Event Log.

This resource manages only the gateway's log provider. When you destroy the link, P0 unlinks the gateway. Use one
link for each gateway. Two links to the same gateway overwrite each other on each apply.

P0 checks that it can reach the gateway through its management tunnel each time the link changes. So the gateway
must be running when you create, change or destroy this resource.

If the gateway is already gone, or P0 cannot reach it, destroy fails. To remove the link from
state without an unlink, run ` + "`terraform state rm p0_ai_gateway_log_provider_link.<name>`" + `.

When a gateway has no log provider, P0 reads its activity from the ` + "`log_project_id`" + ` on
` + "`p0_ai_gateway`" + `, if it has one.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The `id` of the gateway",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"gateway": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the installed `p0_ai_gateway` to link",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"log_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_ai_gateway_log_provider` that holds the gateway's activity logs",
			},
		},
	}
}

func (r *LogProviderLink) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:           IntegrationKey,
		Component:             installresources.Gateway,
		ProviderData:          providerData,
		GetId:                 r.getId,
		GetItemJson:           r.getItemJson,
		FromJson:              r.fromJson,
		ToJson:                r.toJson,
		DescribeCheckError:    describeLinkError,
		DescribeWriteNotFound: describeLinkGatewayNotFound,
	}
}

// describeLinkError describes a refused gateway configure. P0 checks that it can
// reach the gateway on every configure, so the cause is often reachability and
// not the log provider.
func describeLinkError(id string, err error) (string, string) {
	return "Could not link the AI gateway's log provider",
		fmt.Sprintf(`P0 refused to update the log provider of gateway %q. P0 checks that it can reach the gateway through its management tunnel each time the gateway changes. So this error can mean that the gateway is not running, that P0 cannot reach it, or that the gateway is not fully installed, and not that the log provider is wrong.

P0 said: %s`, id, err)
}

// describeUnlinkError describes a refused unlink. It has the same cause as a
// refused link.
func describeUnlinkError(id string, err error) (string, string) {
	_, detail := describeLinkError(id, err)
	return "Could not unlink the AI gateway's log provider", detail
}

func describeLinkGatewayNotFound(id string, err error) (string, string) {
	return "Could not link the AI gateway's log provider",
		fmt.Sprintf("P0 has no gateway %q. Install it with the p0_ai_gateway resource before you link it.\n\nP0 said: %s", id, err)
}

func (r *LogProviderLink) getId(data any) *string {
	model, ok := data.(*logProviderLinkModel)
	if !ok {
		return nil
	}
	if model.Gateway.IsNull() || model.Gateway.IsUnknown() {
		return nil
	}
	id := model.Gateway.ValueString()
	return &id
}

func (r *LogProviderLink) getItemJson(json any) any {
	api, ok := json.(*logProviderLinkApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *LogProviderLink) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*logProviderLinkJson)
	if !ok {
		return nil
	}
	logProvider := ""
	if json.LogProvider != nil {
		logProvider = *json.LogProvider
	}
	return &logProviderLinkModel{
		Id:          types.StringValue(id),
		Gateway:     types.StringValue(id),
		LogProvider: types.StringValue(logProvider),
	}
}

func (r *LogProviderLink) toJson(data any) any {
	model, ok := data.(*logProviderLinkModel)
	if !ok {
		return nil
	}
	return &logProviderLinkConfigureJson{
		LogProvider: model.LogProvider.ValueString(),
		State:       common.Config,
	}
}

func (r *LogProviderLink) upsert(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State) {
	r.installer.UpsertFromConfigure(ctx, diags, plan, state, &logProviderLinkApi{}, &logProviderLinkModel{})
}

func (r *LogProviderLink) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State)
}

// Read removes the link from state when the gateway is gone, or no longer has a
// log provider. The next plan then creates the link again.
func (r *LogProviderLink) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &logProviderLinkApi{}, &logProviderLinkModel{})
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		return
	}

	var stored logProviderLinkModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &stored)...)
	if !resp.Diagnostics.HasError() && stored.LogProvider.ValueString() == "" {
		tflog.Debug(ctx, fmt.Sprintf("Gateway %q has no log provider, removing its link from state", stored.Gateway.ValueString()))
		resp.State.RemoveResource(ctx)
	}
}

func (r *LogProviderLink) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State)
}

// Delete unlinks the gateway. P0 rejects a null log provider, so the request
// sends "". A gateway that is already gone needs no unlink.
func (r *LogProviderLink) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data logProviderLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The same path that common.Install posts the configure step to.
	itemPath := fmt.Sprintf("integrations/%s/config/%s/%s/%s",
		r.installer.Integration, r.installer.Component, data.Gateway.ValueString(), common.Config)
	body := r.toJson(&logProviderLinkModel{
		Id:          data.Id,
		Gateway:     data.Gateway,
		LogProvider: types.StringValue(""),
	})
	httpResp, err := r.installer.ProviderData.Post(itemPath, body, &logProviderLinkApi{})
	if err == nil {
		return
	}

	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	switch status {
	case http.StatusNotFound:
		tflog.Debug(ctx, fmt.Sprintf("Gateway %q not found (404), so it needs no unlink", data.Gateway.ValueString()))
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusBadGateway:
		resp.Diagnostics.AddError(describeUnlinkError(data.Gateway.ValueString(), err))
	default:
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Could not unlink the log provider of gateway %q, got error:\n%s", data.Gateway.ValueString(), err))
	}
}

// ImportState imports a link by its gateway's id.
func (r *LogProviderLink) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("gateway"), req, resp)
}
