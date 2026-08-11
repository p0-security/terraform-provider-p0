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

var _ resource.Resource = &GcpWifIdentityStaged{}
var _ resource.ResourceWithImportState = &GcpWifIdentityStaged{}
var _ resource.ResourceWithConfigure = &GcpWifIdentityStaged{}

func NewGcpWifIdentityStaged() resource.Resource {
	return &GcpWifIdentityStaged{}
}

type GcpWifIdentityStaged struct {
	installer *common.Install
}

type gcpWifIdentityStagedModel struct {
	Id              string       `tfsdk:"id"`
	ProjectId       string       `tfsdk:"project_id"`
	OidcProviderUrl string       `tfsdk:"oidc_provider_url"`
	Audience        types.String `tfsdk:"audience"`
}

type gcpWifIdentityStagedApi struct {
	Item gcpWifIdentityJson `json:"item"`
}

func (r *GcpWifIdentityStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_wif_identity_staged"
}

func (r *GcpWifIdentityStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GCP Workload Identity Federation identity.

P0 computes ` + "`audience`" + ` from ` + "`project_id`" + ` — it's the full resource name of the Workload Identity
Pool provider P0 expects, in the form
` + "`//iam.googleapis.com/projects/{number}/locations/global/workloadIdentityPools/{pool}/providers/{provider}`" + `.
Create a Workload Identity Pool and provider with exactly this name (see the example usage) before
` + "`p0_gcp_wif_identity`" + ` can finish installing; P0 does not create this GCP-side infrastructure on your
behalf.

For instructions on using this resource, see the documentation for ` + "`p0_gcp_wif_identity`" + `.`,
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"oidc_provider_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL of your OIDC provider (e.g. `https://token.actions.githubusercontent.com`)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"audience": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The `aud` claim the identity will send to GCP when calling APIs",
			},
		},
	}
}

func (r *GcpWifIdentityStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.Identity,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
	}
}

func (r *GcpWifIdentityStaged) getId(data any) *string {
	model, ok := data.(*gcpWifIdentityStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *GcpWifIdentityStaged) getItemJson(json any) any {
	api, ok := json.(*gcpWifIdentityStagedApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *GcpWifIdentityStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*gcpWifIdentityJson)
	if !ok {
		return nil
	}
	return &gcpWifIdentityStagedModel{
		Id:              id,
		ProjectId:       json.ProjectId,
		OidcProviderUrl: json.OidcProviderUrl,
		Audience:        types.StringPointerValue(json.Audience),
	}
}

func (r *GcpWifIdentityStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var inputData gcpWifIdentityStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json gcpWifIdentityStagedApi
	var data gcpWifIdentityStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &gcpWifIdentityStageJson{
		ProjectId:       inputData.ProjectId,
		OidcProviderUrl: inputData.OidcProviderUrl,
	})
}

func (r *GcpWifIdentityStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gcpWifIdentityStagedApi
	var data gcpWifIdentityStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *GcpWifIdentityStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var inputData gcpWifIdentityStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json gcpWifIdentityStagedApi
	var data gcpWifIdentityStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &gcpWifIdentityStageJson{
		ProjectId:       inputData.ProjectId,
		OidcProviderUrl: inputData.OidcProviderUrl,
	})
}

func (r *GcpWifIdentityStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gcpWifIdentityStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *GcpWifIdentityStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
