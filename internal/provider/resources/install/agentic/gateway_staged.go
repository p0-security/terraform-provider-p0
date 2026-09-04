// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installagentic

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &GatewayStaged{}
var _ resource.ResourceWithImportState = &GatewayStaged{}
var _ resource.ResourceWithConfigure = &GatewayStaged{}

func NewGatewayStaged() resource.Resource {
	return &GatewayStaged{}
}

type GatewayStaged struct {
	installer *common.Install
}

type gatewayDomainHostingStagedModel struct {
	Type          types.String `tfsdk:"type"`
	Url           string       `tfsdk:"url"`
	OauthEndpoint types.String `tfsdk:"oauth_endpoint"`
}

type gatewayStagedModel struct {
	Id                  string                           `tfsdk:"id"`
	DomainHosting       *gatewayDomainHostingStagedModel `tfsdk:"domain_hosting"`
	LetsEncryptEmail    string                           `tfsdk:"lets_encrypt_email"`
	OidcClientId        string                           `tfsdk:"oidc_client_id"`
	StorageClass        string                           `tfsdk:"storage_class"`
	KubernetesNamespace types.String                     `tfsdk:"kubernetes_namespace"`
	ServiceAccountEmail types.String                     `tfsdk:"service_account_email"`
}

type gatewayStagedApi struct {
	Item gatewayJson `json:"item"`
}

type gatewayDomainHostingStageJson struct {
	Type          string  `json:"type"`
	Url           string  `json:"url"`
	OauthEndpoint *string `json:"oauth-endpoint,omitempty"`
}

type gatewayStageJson struct {
	DomainHosting       gatewayDomainHostingStageJson `json:"domainHosting"`
	LetsEncryptEmail    string                        `json:"letsEncryptEmail"`
	OidcClientId        string                        `json:"oidcClientId"`
	StorageClass        string                        `json:"storageClass"`
	KubernetesNamespace *string                       `json:"kubernetesNamespace,omitempty"`
}

func (r *GatewayStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agentic_gateway_staged"
}

func (r *GatewayStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged installation of an Agentic gateway.

P0 assigns a service account to communicate with your gateway, returned as ` + "`service_account_email`" + `. Your
gateway must be configured to trust this service account (e.g. the ` + "`manageAllowedEmails`" + ` value in the
` + "`agentic-gateway-stack`" + ` Helm chart) before ` + "`p0_agentic_gateway`" + ` can finish installing — P0 cannot
authenticate to your gateway's management API otherwise. See the example usage for the recommended pattern.

For instructions on using this resource, see the documentation for ` + "`p0_agentic_gateway`" + `.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this gateway",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain_hosting": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "How this gateway's public hostname and DNS records are managed.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("selfHosted"),
						MarkdownDescription: "Currently only 'selfHosted' (bring your own domain and manage its DNS records yourself) is supported.",
					},
					"url": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Agentic gateway URL; your servers will be hosted here.",
					},
					"oauth_endpoint": schema.StringAttribute{
						Optional: true,
						Computed: true,
						MarkdownDescription: `OAuth server endpoint; must be publicly accessible and host ` + "`.well-known/jwks.json`" + `. Defaults to
` + "`url`" + ` — only set this if your OAuth/JWKS endpoint runs on a different host than the gateway itself.`,
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"lets_encrypt_email": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Email address used for the ACME account (renewal notifications, account recovery); not shared publicly.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"oidc_client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "OAuth client ID from your upstream OAuth provider (e.g. Google Cloud Console) that end users authenticate against.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"storage_class": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Kubernetes StorageClass your cluster provides for the gateway's bundled PostgreSQL PersistentVolume, e.g. `gp2` (AWS EKS), `standard-rwo` (GKE), or `managed-premium` (AKS).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"kubernetes_namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The namespace this gateway is deployed into. Defaults to `p0-agentic-gateway` if left blank.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Email address of the service account identity that P0 uses to communicate with your gateway",
			},
		},
	}
}

// ToJson is unused: this resource only ever calls installer.Stage (which
// takes its request body directly) and installer.Read/Delete (which don't
// serialize the model at all).
func (r *GatewayStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.Gateway,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
	}
}

func (r *GatewayStaged) getId(data any) *string {
	model, ok := data.(*gatewayStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *GatewayStaged) getItemJson(json any) any {
	api, ok := json.(*gatewayStagedApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *GatewayStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*gatewayJson)
	if !ok {
		return nil
	}
	return &gatewayStagedModel{
		Id: id,
		DomainHosting: &gatewayDomainHostingStagedModel{
			Type:          types.StringValue(json.DomainHosting.Type),
			Url:           json.DomainHosting.Url,
			OauthEndpoint: types.StringPointerValue(json.DomainHosting.OauthEndpoint),
		},
		LetsEncryptEmail:    json.LetsEncryptEmail,
		OidcClientId:        json.OidcClientId,
		StorageClass:        json.StorageClass,
		KubernetesNamespace: types.StringPointerValue(json.KubernetesNamespace),
		ServiceAccountEmail: types.StringPointerValue(json.ServiceAccountEmail),
	}
}

func (inputData *gatewayStagedModel) stageJson() *gatewayStageJson {
	if inputData.DomainHosting == nil {
		return nil
	}
	return &gatewayStageJson{
		DomainHosting: gatewayDomainHostingStageJson{
			Type:          inputData.DomainHosting.Type.ValueString(),
			Url:           inputData.DomainHosting.Url,
			OauthEndpoint: inputData.DomainHosting.OauthEndpoint.ValueStringPointer(),
		},
		LetsEncryptEmail:    inputData.LetsEncryptEmail,
		OidcClientId:        inputData.OidcClientId,
		StorageClass:        inputData.StorageClass,
		KubernetesNamespace: inputData.KubernetesNamespace.ValueStringPointer(),
	}
}

func (r *GatewayStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var inputData gatewayStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}
	stageJson := inputData.stageJson()
	if stageJson == nil {
		resp.Diagnostics.AddError("Missing domain_hosting", "Could not read domain_hosting from plan")
		return
	}

	var json gatewayStagedApi
	var data gatewayStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, stageJson)
}

func (r *GatewayStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gatewayStagedApi
	var data gatewayStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *GatewayStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var inputData gatewayStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}
	stageJson := inputData.stageJson()
	if stageJson == nil {
		resp.Diagnostics.AddError("Missing domain_hosting", "Could not read domain_hosting from plan")
		return
	}

	var json gatewayStagedApi
	var data gatewayStagedModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, stageJson)
}

func (r *GatewayStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gatewayStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *GatewayStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
