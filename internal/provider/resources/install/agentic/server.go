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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &Server{}
var _ resource.ResourceWithImportState = &Server{}
var _ resource.ResourceWithConfigure = &Server{}

func NewServer() resource.Resource {
	return &Server{}
}

type Server struct {
	installer *common.Install
}

// serverCredentialModel and serverDefinitionModel are flattened discriminated
// unions: every variant's fields are declared as optional siblings of the
// `type` (or, for hosting, a second-level) discriminator, matching the
// flattening convention used for `p0_access_policy`'s `requestor.agent`
// (see access_policy/common.go's AgentModel). Their shape mirrors
// app/shared/src/integrations/resources/agentic/components.ts's
// `credentialSource` and `serverDefinition` elements exactly.
type serverCredentialGrantModel struct {
	Type     string  `tfsdk:"type" json:"type"`
	Pkce     *bool   `tfsdk:"pkce" json:"pkce,omitempty"`
	ClientId *string `tfsdk:"client_id" json:"clientId,omitempty"`
}

type serverCredentialModel struct {
	Type     string                      `tfsdk:"type" json:"type"`
	Provider *string                     `tfsdk:"provider" json:"provider,omitempty"`
	Grant    *serverCredentialGrantModel `tfsdk:"grant" json:"grant,omitempty"`
}

// serverDefinitionHostingModel is itself a discriminated union (container vs
// external), so — like serverCredentialGrantModel — it stays a nested object
// rather than flattening onto serverDefinitionModel: the backend rejects a
// flat `hosting` string with "'definition/custom/hosting' must be a map with
// a string 'type' property".
type serverDefinitionHostingModel struct {
	Type       string  `tfsdk:"type" json:"type"`
	Entrypoint *string `tfsdk:"entrypoint" json:"entrypoint,omitempty"`
	Image      *string `tfsdk:"image" json:"image,omitempty"`
	Url        *string `tfsdk:"url" json:"url,omitempty"`
	Label      *string `tfsdk:"label" json:"label,omitempty"`
}

type serverDefinitionModel struct {
	Type    string                        `tfsdk:"type" json:"type"`
	Id      *string                       `tfsdk:"id" json:"id,omitempty"`
	Hosting *serverDefinitionHostingModel `tfsdk:"hosting" json:"hosting,omitempty"`
	LogoUrl *string                       `tfsdk:"logo_url" json:"logoUrl,omitempty"`
	Prompt  *string                       `tfsdk:"prompt" json:"prompt,omitempty"`
}

type serverModel struct {
	Id         string                 `tfsdk:"id"`
	Gateway    types.String           `tfsdk:"gateway"`
	Credential *serverCredentialModel `tfsdk:"credential"`
	Definition *serverDefinitionModel `tfsdk:"definition"`
}

type serverJson struct {
	Gateway    string                `json:"gateway"`
	Credential serverCredentialModel `json:"credential"`
	Definition serverDefinitionModel `json:"definition"`
	State      string                `json:"state"`
}

type serverApi struct {
	Item serverJson `json:"item"`
}

// serverStageJson carries only the "step: new" `gateway` field (see
// app/shared/src/integrations/resources/agentic/components.ts's
// `gatewaySelect` element); resending it from the later verify/configure
// calls (see toJson) is rejected by the backend with "can only be altered on
// initial installation".
type serverStageJson struct {
	Gateway string `json:"gateway"`
}

// serverConfigureJson carries the mutable fields sent by toJson, used for the
// verify/configure calls issued by UpsertFromStage.
type serverConfigureJson struct {
	Credential serverCredentialModel `json:"credential"`
	Definition serverDefinitionModel `json:"definition"`
	State      string                `json:"state"`
}

func (r *Server) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agentic_server"
}

func (r *Server) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Registers an MCP server behind an Agentic gateway, giving agents access to your resources.

To reference an AWS- or GCP-federated credential, install the corresponding ` + "`p0_aws_oidc_identity`" + ` or
` + "`p0_gcp_wif_identity`" + ` resource first and pass its ` + "`id`" + ` as ` + "`credential.provider`" + `.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this MCP server",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gateway": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_agentic_gateway` that hosts this server",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"credential": schema.SingleNestedAttribute{
				Required: true,
				MarkdownDescription: `The MCP server's credential source:
    - 'aws': federate credentials from an installed AWS OIDC identity
    - 'gcp': federate credentials from an installed GCP WIF identity
    - 'oauth': authenticate end users via an OAuth session`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "One of 'aws', 'gcp', or 'oauth'.",
					},
					"provider": schema.StringAttribute{
						Optional: true,
						MarkdownDescription: `Required, and may only be used, if 'type' is 'aws' or 'gcp'. The ` + "`id`" + ` of the installed
federation-provider identity (a ` + "`p0_aws_oidc_identity`" + ` or ` + "`p0_gcp_wif_identity`" + ` resource).`,
					},
					"grant": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Required, and may only be used, if 'type' is 'oauth'. The OAuth grant used to obtain the credential.",
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "The OAuth grant type. Currently only 'authorization_code' is supported.",
							},
							"pkce": schema.BoolAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if grant 'type' is 'authorization_code'. Whether Proof Key for Code Exchange (PKCE) is used.",
							},
							"client_id": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if grant 'type' is 'authorization_code'. OAuth client identifier registered with the upstream provider.",
							},
						},
						Validators: []validator.Object{
							RequiredWhenAttr("type", map[string][]string{
								"authorization_code": {"pkce", "client_id"},
							}),
							ExclusiveToAttr("type", map[string][]string{
								"authorization_code": {"pkce", "client_id"},
							}),
						},
					},
				},
				Validators: []validator.Object{
					RequiredWhenAttr("type", map[string][]string{
						"aws":   {"provider"},
						"gcp":   {"provider"},
						"oauth": {"grant"},
					}),
					ExclusiveToAttr("type", map[string][]string{
						"aws":   {"provider"},
						"gcp":   {"provider"},
						"oauth": {"grant"},
					}),
				},
			},
			"definition": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Whether this server is predefined by P0, or a custom server definition.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "One of 'p0' or 'custom'.",
					},
					"id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Required, and may only be used, if 'type' is 'p0'. Pre-defined server identifier.",
					},
					"hosting": schema.SingleNestedAttribute{
						Optional: true,
						MarkdownDescription: `Required, and may only be used, if 'type' is 'custom'. Whether this server proxies to an
externally hosted server ('external') or hosts a containerized server ('container').`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "One of 'container' or 'external'.",
							},
							"entrypoint": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if 'type' is 'container'. Container run entrypoint; use jinja2 syntax to template request parameters and credentials.",
							},
							"image": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if 'type' is 'container'. Image that hosts the MCP server.",
							},
							"url": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if 'type' is 'external'. URL of the externally hosted MCP server.",
							},
							"label": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Required, and may only be used, if 'type' is 'external'. Human-friendly label for this server.",
							},
						},
						Validators: []validator.Object{
							RequiredWhenAttr("type", map[string][]string{
								"container": {"entrypoint", "image"},
								"external":  {"url", "label"},
							}),
							ExclusiveToAttr("type", map[string][]string{
								"container": {"entrypoint", "image"},
								"external":  {"url", "label"},
							}),
						},
					},
					"logo_url": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "May only be used if 'type' is 'custom'. An address of a logo image for the server.",
					},
					"prompt": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Required, and may only be used, if 'type' is 'custom'. Text used to describe this server to agents.",
					},
				},
				Validators: []validator.Object{
					RequiredWhenAttr("type", map[string][]string{
						"p0":     {"id"},
						"custom": {"hosting", "prompt"},
					}),
					ExclusiveToAttr("type", map[string][]string{
						"p0":     {"id"},
						"custom": {"hosting", "prompt", "logo_url"},
					}),
				},
			},
		},
	}
}

func (r *Server) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.Server,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *Server) getId(data any) *string {
	model, ok := data.(*serverModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *Server) getItemJson(json any) any {
	api, ok := json.(*serverApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *Server) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*serverJson)
	if !ok {
		return nil
	}
	credential := json.Credential
	definition := json.Definition
	return &serverModel{
		Id:         id,
		Gateway:    types.StringValue(json.Gateway),
		Credential: &credential,
		Definition: &definition,
	}
}

func (r *Server) toJson(data any) any {
	model, ok := data.(*serverModel)
	if !ok || model.Credential == nil || model.Definition == nil {
		return nil
	}
	return &serverConfigureJson{
		Credential: *model.Credential,
		Definition: *model.Definition,
		State:      common.Config,
	}
}

func (r *Server) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &serverModel{})

	var inputData serverModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json serverApi
	var data serverModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &serverStageJson{Gateway: inputData.Gateway.ValueString()})
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Server) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json serverApi
	var data serverModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *Server) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &serverModel{})
	var json serverApi
	var data serverModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Server) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serverModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *Server) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
