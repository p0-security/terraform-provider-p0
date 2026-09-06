// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installagentic

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

type gatewayDomainHostingModel struct {
	LoadBalancerIp types.String `tfsdk:"load_balancer_ip"`
}

// gatewayDomainHostingAttrTypes describes gatewayDomainHostingModel's shape
// for conversion to/from types.Object — required because domain_hosting is
// Optional+Computed, so its value may be Unknown at plan time (e.g. omitted
// from config on Create); a plain Go struct/pointer field can't represent
// that, only a framework attr.Value type can.
var gatewayDomainHostingAttrTypes = map[string]attr.Type{
	"load_balancer_ip": types.StringType,
}

type gatewayModel struct {
	Id                  string       `tfsdk:"id"`
	DomainHosting       types.Object `tfsdk:"domain_hosting"`
	LogProjectId        types.String `tfsdk:"log_project_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
}

type gatewayDomainHostingJson struct {
	Type           string  `json:"type"`
	Url            string  `json:"url"`
	OauthEndpoint  *string `json:"oauth-endpoint,omitempty"`
	LoadBalancerIp *string `json:"loadBalancerIp,omitempty"`
}

// gatewayJson is shared with gateway_staged.go: both resources read the same
// backend item via GET, so its shape is defined once here rather than
// duplicated per resource (see gatewayStagedApi in gateway_staged.go).
type gatewayJson struct {
	DomainHosting       gatewayDomainHostingJson `json:"domainHosting"`
	LetsEncryptEmail    string                   `json:"letsEncryptEmail"`
	OidcClientId        string                   `json:"oidcClientId"`
	StorageClass        string                   `json:"storageClass"`
	KubernetesNamespace *string                  `json:"kubernetesNamespace,omitempty"`
	LogProjectId        *string                  `json:"log-project-id,omitempty"`
	ServiceAccountEmail *string                  `json:"serviceAccountEmail,omitempty"`
	State               string                   `json:"state"`
}

type gatewayApi struct {
	Item gatewayJson `json:"item"`
}

type gatewayDomainHostingConfigureJson struct {
	Type           string  `json:"type"`
	LoadBalancerIp *string `json:"loadBalancerIp,omitempty"`
}

type gatewayConfigureJson struct {
	DomainHosting *gatewayDomainHostingConfigureJson `json:"domainHosting,omitempty"`
	LogProjectId  *string                            `json:"log-project-id,omitempty"`
	State         string                             `json:"state"`
}

func (r *Gateway) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agentic_gateway"
}

func (r *Gateway) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final installation of an Agentic gateway, which hosts MCP servers and applies P0 access
policy to agent tool calls.

To use this resource, you must also install the ` + "`p0_agentic_gateway_staged`" + ` resource, and configure your
gateway to trust the service account returned by that resource (e.g. the ` + "`manageAllowedEmails`" + ` value in the
` + "`agentic-gateway-stack`" + ` Helm chart).

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_agentic_gateway_staged` resource being finalized",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain_hosting": schema.SingleNestedAttribute{
				Optional: true,
				// Computed: fromJson always populates this from the backend's
				// response (which always returns a domainHosting object), even
				// when config omits it entirely — without Computed, that would
				// mismatch the planned null value and fail Terraform's
				// post-apply consistency check.
				Computed:            true,
				MarkdownDescription: "How this gateway's public hostname and DNS records are managed.",
				Attributes: map[string]schema.Attribute{
					"load_balancer_ip": schema.StringAttribute{
						Optional: true,
						MarkdownDescription: `Your gateway's LoadBalancer address (IP or hostname). Create a DNS record for the ` + "`url`" + ` on the
` + "`p0_agentic_gateway_staged`" + ` resource pointing here: an A record for an IPv4 address, AAAA for IPv6, or CNAME
if your cloud provider (e.g. AWS) gave you a hostname instead of a static IP.`,
					},
				},
			},
			"log_project_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `GCP project ID where this gateway's Cloud Logging entries actually land, so P0 can show its
MCP tool-call and A2A activity. This is the project holding the log bucket, which may not be the same project the
gateway itself runs in (e.g. if logs are routed to a centralized logging project).`,
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
	domainHosting, dhDiags := types.ObjectValueFrom(ctx, gatewayDomainHostingAttrTypes, gatewayDomainHostingModel{
		LoadBalancerIp: types.StringPointerValue(json.DomainHosting.LoadBalancerIp),
	})
	diags.Append(dhDiags...)
	if dhDiags.HasError() {
		return nil
	}
	return &gatewayModel{
		Id:                  id,
		DomainHosting:       domainHosting,
		LogProjectId:        types.StringPointerValue(json.LogProjectId),
		ServiceAccountEmail: types.StringPointerValue(json.ServiceAccountEmail),
	}
}

func (r *Gateway) toJson(data any) any {
	model, ok := data.(*gatewayModel)
	if !ok {
		return nil
	}
	var domainHosting *gatewayDomainHostingConfigureJson
	// Unknown when config omits domain_hosting entirely on Create (it's
	// Optional+Computed, so ToJson — called against the plan, before the
	// backend has computed a value — sees Unknown rather than Null here).
	if !model.DomainHosting.IsNull() && !model.DomainHosting.IsUnknown() {
		var dh gatewayDomainHostingModel
		asDiags := model.DomainHosting.As(context.Background(), &dh, basetypes.ObjectAsOptions{})
		if asDiags.HasError() {
			return nil
		}
		var loadBalancerIp *string
		if !dh.LoadBalancerIp.IsNull() && !dh.LoadBalancerIp.IsUnknown() {
			loadBalancerIp = dh.LoadBalancerIp.ValueStringPointer()
		}
		domainHosting = &gatewayDomainHostingConfigureJson{
			Type:           "selfHosted",
			LoadBalancerIp: loadBalancerIp,
		}
	}
	return &gatewayConfigureJson{
		DomainHosting: domainHosting,
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
