// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installgcpwif

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

const IntegrationKey = "gcp-wif"

var _ resource.Resource = &GcpWifIdentity{}
var _ resource.ResourceWithImportState = &GcpWifIdentity{}
var _ resource.ResourceWithConfigure = &GcpWifIdentity{}

func NewGcpWifIdentity() resource.Resource {
	return &GcpWifIdentity{}
}

type GcpWifIdentity struct {
	installer *common.Install
}

type gcpWifIdentityModel struct {
	Id              string       `tfsdk:"id"`
	ProjectId       types.String `tfsdk:"project_id"`
	OidcProviderUrl types.String `tfsdk:"oidc_provider_url"`
	Audience        types.String `tfsdk:"audience"`
}

type gcpWifIdentityJson struct {
	ProjectId       string  `json:"projectId"`
	OidcProviderUrl string  `json:"oidcProviderUrl"`
	Audience        *string `json:"audience,omitempty"`
	State           string  `json:"state"`
}

type gcpWifIdentityApi struct {
	Item gcpWifIdentityJson `json:"item"`
}

// gcpWifIdentityStageJson carries the "step: new" fields (see
// app/shared/src/integrations/resources/gcp-wif/components.ts's `identity`
// component, where `projectId` and `oidcProviderUrl` are `step: "new"`);
// resending them from the later verify/configure calls (see toJson) is
// rejected by the backend with "can only be altered on initial
// installation". `audience` is `type: "generated"` and never sent by us —
// that's why these fields are only settable on p0_gcp_wif_identity_staged.
type gcpWifIdentityStageJson struct {
	ProjectId       string `json:"projectId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
}

// gcpWifIdentityConfigureJson carries the payload sent by toJson for the
// verify/configure calls issued by UpsertFromStage (and the PUT issued by
// Rollback on delete). `projectId`/`oidcProviderUrl` are resent here even
// though they're `step: "new"` — the same, unchanged values the user already
// staged — because omitting them would silently drop the stored values from
// the merged item (the `audience` field's assembler reads the raw unmerged
// request body, not the merged one) rather than just failing to change them;
// this mirrors p0_okta_directory_listing's final resource, which resends its
// own immutable fields the same way.
type gcpWifIdentityConfigureJson struct {
	ProjectId       string `json:"projectId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
	State           string `json:"state"`
}

func (r *GcpWifIdentity) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_wif_identity"
}

func (r *GcpWifIdentity) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final installation of a GCP Workload Identity Federation identity, allowing P0 to grant and
revoke GCP access for OIDC-authenticated agents (e.g. CI/CD pipelines).

To use this resource, you must also:
- install the ` + "`p0_gcp_wif_identity_staged`" + ` resource, and
- create a Workload Identity Pool and provider matching the ` + "`audience`" + ` from that resource
  (` + "`google_iam_workload_identity_pool`" + ` and ` + "`google_iam_workload_identity_pool_provider`" + `).

Requires the Google Cloud integration (` + "`p0_gcp`" + `) to be installed for the same project. See the example
usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_gcp_wif_identity_staged` resource being finalized",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GCP project ID to federate access into. Must match `project_id` on the `p0_gcp_wif_identity_staged` resource.",
			},
			"oidc_provider_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL of your OIDC provider. Must match `oidc_provider_url` on the `p0_gcp_wif_identity_staged` resource.",
			},
			"audience": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The `aud` claim the identity will send to GCP when calling APIs",
			},
		},
	}
}

func (r *GcpWifIdentity) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.Identity,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GcpWifIdentity) getId(data any) *string {
	model, ok := data.(*gcpWifIdentityModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *GcpWifIdentity) getItemJson(json any) any {
	api, ok := json.(*gcpWifIdentityApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *GcpWifIdentity) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*gcpWifIdentityJson)
	if !ok {
		return nil
	}
	return &gcpWifIdentityModel{
		Id:              id,
		ProjectId:       types.StringValue(json.ProjectId),
		OidcProviderUrl: types.StringValue(json.OidcProviderUrl),
		Audience:        types.StringPointerValue(json.Audience),
	}
}

func (r *GcpWifIdentity) toJson(data any) any {
	model, ok := data.(*gcpWifIdentityModel)
	if !ok {
		return nil
	}
	return &gcpWifIdentityConfigureJson{
		ProjectId:       model.ProjectId.ValueString(),
		OidcProviderUrl: model.OidcProviderUrl.ValueString(),
		State:           common.Config,
	}
}

func (r *GcpWifIdentity) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gcpWifIdentityModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *GcpWifIdentity) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
