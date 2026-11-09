// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installawsfederation

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

const (
	IntegrationKey = "aws-federation"
	Component      = "federation"
)

var federationIdRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var _ resource.Resource = &AwsFederation{}
var _ resource.ResourceWithImportState = &AwsFederation{}
var _ resource.ResourceWithConfigure = &AwsFederation{}

func NewAwsFederation() resource.Resource {
	return &AwsFederation{}
}

type AwsFederation struct {
	installer *common.Install
}

type awsFederationOktaModel struct {
	AppId           types.String `tfsdk:"app_id"`
	RoleLabelFormat types.String `tfsdk:"role_label_format"`
}

type awsFederationModel struct {
	Id    string                  `tfsdk:"id"`
	Okta  *awsFederationOktaModel `tfsdk:"okta"`
	State types.String            `tfsdk:"state"`
}

type awsFederationTypeJson struct {
	Type string `json:"type"`
}

type awsFederationProviderJson struct {
	Type            string                `json:"type"`
	AppId           string                `json:"appId"`
	RoleLabelFormat awsFederationTypeJson `json:"roleLabelFormat"`
}

type awsFederationJson struct {
	Provider *awsFederationProviderJson `json:"provider,omitempty"`
	State    string                     `json:"state,omitempty"`
}

type awsFederationApi struct {
	Item awsFederationJson `json:"item"`
}

func (r *AwsFederation) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_federation"
}

func (r *AwsFederation) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS federation app: one app in an identity provider that signs users in to AWS accounts.

AWS accounts use the app by setting 'login.federation' on their 'p0_aws_iam_write' resource. P0 needs the identity
provider's directory integration (for example 'p0_okta_directory_listing') to assign AWS roles to users in the app.

**Preview**: This resource may change.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this federation app (lowercase letters, digits, and hyphens)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(federationIdRegex, "Use only lowercase letters, digits, and hyphens, and start with a letter or a digit"),
				},
			},
			"okta": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "The federation app in Okta",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"app_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Okta AWS federation app ID",
						Validators:          []validator.String{stringvalidator.RegexMatches(installaws.OktaAppIdRegex, "Okta app IDs should start with '0o'")},
					},
					"role_label_format": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("role-name"),
						MarkdownDescription: `How the federation app names an AWS role:
    - 'role-name': The app signs in to one AWS account (default)
    - 'alias-and-role-name': The app signs in to several AWS accounts, and names roles "[alias] -- role"`,
						Validators: []validator.String{stringvalidator.OneOf("role-name", "alias-and-role-name")},
					},
				},
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *AwsFederation) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    Component,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsFederation) getId(data any) *string {
	model, ok := data.(*awsFederationModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsFederation) getItemJson(json any) any {
	api, ok := json.(*awsFederationApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *AwsFederation) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*awsFederationJson)
	if !ok {
		return nil
	}
	data := awsFederationModel{Id: id, State: types.StringValue(json.State)}
	if json.Provider != nil && json.Provider.Type == "okta" {
		data.Okta = &awsFederationOktaModel{
			AppId:           types.StringValue(json.Provider.AppId),
			RoleLabelFormat: types.StringValue(json.Provider.RoleLabelFormat.Type),
		}
	}
	return &data
}

func (r *AwsFederation) toJson(data any) any {
	model, ok := data.(*awsFederationModel)
	if !ok {
		return nil
	}
	json := awsFederationJson{}
	if model.Okta != nil {
		json.Provider = &awsFederationProviderJson{
			Type:            "okta",
			AppId:           model.Okta.AppId.ValueString(),
			RoleLabelFormat: awsFederationTypeJson{Type: model.Okta.RoleLabelFormat.ValueString()},
		}
	}
	return &json
}

func (r *AwsFederation) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data awsFederationModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// The backend accepts the provider only on the first step
	var json awsFederationApi
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsFederation) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsFederationApi
	var data awsFederationModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsFederation) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsFederationApi
	var data awsFederationModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsFederation) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsFederationModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *AwsFederation) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
