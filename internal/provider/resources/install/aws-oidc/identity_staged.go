// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installawsoidc

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

var _ resource.Resource = &AwsOidcIdentityStaged{}
var _ resource.ResourceWithImportState = &AwsOidcIdentityStaged{}
var _ resource.ResourceWithConfigure = &AwsOidcIdentityStaged{}

func NewAwsOidcIdentityStaged() resource.Resource {
	return &AwsOidcIdentityStaged{}
}

type AwsOidcIdentityStaged struct {
	installer *common.Install
}

type awsOidcIdentityStagedModel struct {
	Id              string       `tfsdk:"id"`
	AccountId       types.String `tfsdk:"account_id"`
	AwsPartition    types.String `tfsdk:"aws_partition"`
	OidcProviderUrl types.String `tfsdk:"oidc_provider_url"`
	Audience        types.String `tfsdk:"audience"`
}

type awsOidcIdentityStagedApi struct {
	Item awsOidcIdentityJson `json:"item"`
}

func (r *AwsOidcIdentityStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_oidc_identity_staged"
}

func (r *AwsOidcIdentityStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS OIDC identity federation.

P0 computes ` + "`aws_partition`" + ` from ` + "`account_id`" + `. Use it, along with ` + "`oidc_provider_url`" + ` and
` + "`audience`" + `, to create the AWS-side IAM OIDC identity provider and the role(s) P0 federates into (P0 does
not create this AWS-side infrastructure on your behalf). Only once that infrastructure exists can
` + "`p0_aws_oidc_identity`" + ` finish installing.

For instructions on using this resource, see the documentation for ` + "`p0_aws_oidc_identity`" + `.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this identity",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS account ID to federate access into",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"aws_partition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The AWS partition (e.g. `aws`, `aws-us-gov`) that the account belongs to",
			},
			"oidc_provider_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL of your OIDC provider (e.g. `https://token.actions.githubusercontent.com`)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"audience": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `aud` claim the identity will send to AWS when calling APIs",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *AwsOidcIdentityStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AwsOidcIdentityStaged) getId(data any) *string {
	model, ok := data.(*awsOidcIdentityStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsOidcIdentityStaged) getItemJson(json any) any {
	api, ok := json.(*awsOidcIdentityStagedApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *AwsOidcIdentityStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*awsOidcIdentityJson)
	if !ok {
		return nil
	}
	return &awsOidcIdentityStagedModel{
		Id:              id,
		AccountId:       types.StringValue(json.AccountId),
		AwsPartition:    types.StringPointerValue(json.AwsPartition),
		OidcProviderUrl: types.StringValue(json.OidcProviderUrl),
		Audience:        types.StringValue(json.Audience),
	}
}

func (r *AwsOidcIdentityStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var inputData awsOidcIdentityStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json awsOidcIdentityStagedApi
	var data awsOidcIdentityStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &awsOidcIdentityStageJson{
		AccountId:       inputData.AccountId.ValueString(),
		OidcProviderUrl: inputData.OidcProviderUrl.ValueString(),
		Audience:        inputData.Audience.ValueString(),
	})
}

func (r *AwsOidcIdentityStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsOidcIdentityStagedApi
	var data awsOidcIdentityStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsOidcIdentityStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var inputData awsOidcIdentityStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json awsOidcIdentityStagedApi
	var data awsOidcIdentityStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &awsOidcIdentityStageJson{
		AccountId:       inputData.AccountId.ValueString(),
		OidcProviderUrl: inputData.OidcProviderUrl.ValueString(),
		Audience:        inputData.Audience.ValueString(),
	})
}

func (r *AwsOidcIdentityStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsOidcIdentityStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *AwsOidcIdentityStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
