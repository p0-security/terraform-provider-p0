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

var _ resource.Resource = &IdentityProvider{}
var _ resource.ResourceWithImportState = &IdentityProvider{}
var _ resource.ResourceWithConfigure = &IdentityProvider{}

func NewIdentityProvider() resource.Resource {
	return &IdentityProvider{}
}

type IdentityProvider struct {
	installer *common.Install
}

type identityProviderModel struct {
	Id                  string       `tfsdk:"id"`
	Issuer              types.String `tfsdk:"issuer"`
	AudiencePattern     types.String `tfsdk:"audience_pattern"`
	SubjectPattern      types.String `tfsdk:"subject_pattern"`
	DynamicRegistration types.Bool   `tfsdk:"dynamic_registration"`
}

type identityProviderJson struct {
	Issuer              string  `json:"issuer"`
	AudiencePattern     *string `json:"audiencePattern,omitempty"`
	SubjectPattern      *string `json:"subjectPattern,omitempty"`
	DynamicRegistration *bool   `json:"dynamicRegistration,omitempty"`
	State               string  `json:"state"`
}

type identityProviderApi struct {
	Item identityProviderJson `json:"item"`
}

// identityProviderStageJson carries only the "step: new" `issuer` field (see
// app/shared/src/integrations/resources/agentic/components.ts's
// `identityProvider` component); resending it from the later verify/configure
// calls (see toJson) is rejected by the backend with "can only be altered on
// initial installation".
type identityProviderStageJson struct {
	Issuer string `json:"issuer"`
}

// identityProviderConfigureJson carries the mutable fields sent by toJson,
// used for the verify/configure calls issued by UpsertFromStage.
type identityProviderConfigureJson struct {
	AudiencePattern     *string `json:"audiencePattern,omitempty"`
	SubjectPattern      *string `json:"subjectPattern,omitempty"`
	DynamicRegistration *bool   `json:"dynamicRegistration,omitempty"`
	State               string  `json:"state"`
}

func (r *IdentityProvider) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agentic_identity_provider"
}

func (r *IdentityProvider) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Enrolls an identity provider whose JWT-authenticated agents may access an Agentic gateway.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this identity provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL (the `iss` claim) of the identity provider to enroll",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"audience_pattern": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pattern that a token's audience (the `aud` claim) must match to be accepted",
			},
			"subject_pattern": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pattern that a token's subject (the `sub` claim) must match to be accepted (omit to accept any subject)",
			},
			"dynamic_registration": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: `If set, identities matching this provider will automatically be registered with your gateways;
otherwise, identities must be manually pre-registered`,
			},
		},
	}
}

func (r *IdentityProvider) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    installresources.IdentityProvider,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *IdentityProvider) getId(data any) *string {
	model, ok := data.(*identityProviderModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *IdentityProvider) getItemJson(json any) any {
	api, ok := json.(*identityProviderApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *IdentityProvider) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*identityProviderJson)
	if !ok {
		return nil
	}
	return &identityProviderModel{
		Id:                  id,
		Issuer:              types.StringValue(json.Issuer),
		AudiencePattern:     types.StringPointerValue(json.AudiencePattern),
		SubjectPattern:      types.StringPointerValue(json.SubjectPattern),
		DynamicRegistration: types.BoolPointerValue(json.DynamicRegistration),
	}
}

func (r *IdentityProvider) toJson(data any) any {
	model, ok := data.(*identityProviderModel)
	if !ok {
		return nil
	}
	return &identityProviderConfigureJson{
		AudiencePattern:     model.AudiencePattern.ValueStringPointer(),
		SubjectPattern:      model.SubjectPattern.ValueStringPointer(),
		DynamicRegistration: model.DynamicRegistration.ValueBoolPointer(),
		State:               common.Config,
	}
}

func (r *IdentityProvider) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &identityProviderModel{})

	var inputData identityProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json identityProviderApi
	var data identityProviderModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &identityProviderStageJson{Issuer: inputData.Issuer.ValueString()})
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *IdentityProvider) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json identityProviderApi
	var data identityProviderModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *IdentityProvider) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &identityProviderModel{})
	var json identityProviderApi
	var data identityProviderModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *IdentityProvider) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data identityProviderModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *IdentityProvider) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
