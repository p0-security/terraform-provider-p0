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
	ProjectId       string       `tfsdk:"project_id"`
	OidcProviderUrl string       `tfsdk:"oidc_provider_url"`
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
// installation." `audience` is `type: "generated"` and never sent by us.
type gcpWifIdentityStageJson struct {
	ProjectId       string `json:"projectId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
}

// gcpWifIdentityConfigureJson carries the (empty) payload sent by toJson for
// the verify/configure calls issued by UpsertFromStage.
type gcpWifIdentityConfigureJson struct {
	State string `json:"state"`
}

func (r *GcpWifIdentity) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_wif_identity"
}

func (r *GcpWifIdentity) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Allows P0 to grant and revoke GCP access for OIDC-authenticated agents (e.g. CI/CD pipelines), via
a Workload Identity Federation pool on your GCP project. Requires the Google Cloud integration (` + "`p0_gcp`" + `)
to be installed for the same project.

P0 uses ` + "`oidc_provider_url`" + ` to generate gcloud CLI/Terraform instructions for creating the Workload
Identity Pool and provider; it does not create the GCP-side resources on your behalf. ` + "`audience`" + ` is
computed by P0 from ` + "`project_id`" + ` once the identity is staged.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this identity",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GCP project ID to federate access into",
			},
			"oidc_provider_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL of your OIDC provider (e.g. `https://token.actions.githubusercontent.com`)",
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
		ProjectId:       json.ProjectId,
		OidcProviderUrl: json.OidcProviderUrl,
		Audience:        types.StringPointerValue(json.Audience),
	}
}

func (r *GcpWifIdentity) toJson(data any) any {
	_, ok := data.(*gcpWifIdentityModel)
	if !ok {
		return nil
	}
	return &gcpWifIdentityConfigureJson{
		State: common.Config,
	}
}

func (r *GcpWifIdentity) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpWifIdentityModel{})

	var inputData gcpWifIdentityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &gcpWifIdentityStageJson{
		ProjectId:       inputData.ProjectId,
		OidcProviderUrl: inputData.OidcProviderUrl,
	})
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpWifIdentityModel{})
	var json gcpWifIdentityApi
	var data gcpWifIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpWifIdentity) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gcpWifIdentityModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *GcpWifIdentity) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
