// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installagentic

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

const IntegrationKey = "agentic"

var _ resource.Resource = &Gateway{}
var _ resource.ResourceWithImportState = &Gateway{}
var _ resource.ResourceWithConfigure = &Gateway{}

func NewGateway() resource.Resource {
	return &Gateway{}
}

type Gateway struct {
	installer *common.Install
}

type gatewayModel struct {
	Id                  string       `tfsdk:"id"`
	Url                 types.String `tfsdk:"url"`
	OauthEndpoint       string       `tfsdk:"oauth_endpoint"`
	LogProjectId        types.String `tfsdk:"log_project_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
}

type gatewayJson struct {
	Url                 string  `json:"url"`
	OauthEndpoint       string  `json:"oauth-endpoint"`
	LogProjectId        *string `json:"log-project-id,omitempty"`
	ServiceAccountEmail *string `json:"serviceAccountEmail,omitempty"`
	State               string  `json:"state"`
}

type gatewayApi struct {
	Item gatewayJson `json:"item"`
}

// gatewayStageJson carries only the "step: new" fields from the DSL (see
// app/shared/src/integrations/resources/agentic/components.ts's `gateway`
// component): fields tagged `step: "new"` may only be sent on the initial
// PUT (stage) call — that's why `url` is owned by p0_agentic_gateway_staged,
// not this resource; resending it from the verify/configure calls (see
// toJson) is rejected by the backend with "can only be altered on initial
// installation."
type gatewayStageJson struct {
	Url string `json:"url"`
}

// gatewayConfigureJson carries the mutable fields sent by toJson, used for
// the verify/configure calls issued by UpsertFromStage.
type gatewayConfigureJson struct {
	OauthEndpoint string  `json:"oauth-endpoint"`
	LogProjectId  *string `json:"log-project-id,omitempty"`
	State         string  `json:"state"`
}

func (r *Gateway) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agentic_gateway"
}

func (r *Gateway) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final installation of an Agentic gateway, which hosts MCP servers and applies P0 access
policy to agent tool calls.

To use this resource, you must also:
- install the ` + "`p0_agentic_gateway_staged`" + ` resource, and
- configure your gateway to trust the service account returned by that resource (e.g. the
  ` + "`manageAllowedEmails`" + ` value in the ` + "`oauthed-mcp-tools`" + ` Helm chart).

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_agentic_gateway_staged` resource being finalized",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Agentic gateway URL; your servers will be hosted here",
			},
			"oauth_endpoint": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "OAuth server endpoint; must be publicly accessible and host `.well-known/jwks.json`",
			},
			"log_project_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `GCP project ID where this gateway's Cloud Logging entries actually land, so P0 can show its
MCP tool-call activity. This is the project holding the log bucket, which may not be the same project the gateway
itself runs in (e.g. if logs are routed to a centralized logging project).`,
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Email address of the service account identity that P0 uses to communicate with your gateway",
			},
		},
	}
}

func (r *Gateway) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.Gateway,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *Gateway) getId(data any) *string {
	model, ok := data.(*gatewayModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *Gateway) getItemJson(json any) any {
	api, ok := json.(*gatewayApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *Gateway) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*gatewayJson)
	if !ok {
		return nil
	}
	return &gatewayModel{
		Id:                  id,
		Url:                 types.StringValue(json.Url),
		OauthEndpoint:       json.OauthEndpoint,
		LogProjectId:        types.StringPointerValue(json.LogProjectId),
		ServiceAccountEmail: types.StringPointerValue(json.ServiceAccountEmail),
	}
}

func (r *Gateway) toJson(data any) any {
	model, ok := data.(*gatewayModel)
	if !ok {
		return nil
	}
	return &gatewayConfigureJson{
		OauthEndpoint: model.OauthEndpoint,
		LogProjectId:  model.LogProjectId.ValueStringPointer(),
		State:         common.Config,
	}
}

func (r *Gateway) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gatewayApi
	var data gatewayModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Gateway) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gatewayApi
	var data gatewayModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *Gateway) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gatewayApi
	var data gatewayModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Gateway) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gatewayModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *Gateway) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
