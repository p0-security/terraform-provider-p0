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

const IntegrationKey = "aws-oidc"

var _ resource.Resource = &AwsOidcIdentity{}
var _ resource.ResourceWithImportState = &AwsOidcIdentity{}
var _ resource.ResourceWithConfigure = &AwsOidcIdentity{}

func NewAwsOidcIdentity() resource.Resource {
	return &AwsOidcIdentity{}
}

type AwsOidcIdentity struct {
	installer *common.Install
}

type awsOidcIdentityModel struct {
	Id              string       `tfsdk:"id"`
	AccountId       string       `tfsdk:"account_id"`
	AwsPartition    types.String `tfsdk:"aws_partition"`
	OidcProviderUrl string       `tfsdk:"oidc_provider_url"`
	Audience        string       `tfsdk:"audience"`
}

type awsOidcIdentityJson struct {
	AccountId       string  `json:"accountId"`
	AwsPartition    *string `json:"awsPartition"`
	OidcProviderUrl string  `json:"oidcProviderUrl"`
	Audience        string  `json:"audience"`
	State           string  `json:"state"`
}

type awsOidcIdentityApi struct {
	Item awsOidcIdentityJson `json:"item"`
}

// awsOidcIdentityStageJson carries the "step: new" fields (see
// app/shared/src/integrations/resources/aws-oidc/components.ts's `identity`
// component, where `accountId`, `oidcProviderUrl`, and `audience` are all
// `step: "new"`); resending them from the later verify/configure calls (see
// toJson) is rejected by the backend with "can only be altered on initial
// installation." None of this component's fields are mutable after creation.
type awsOidcIdentityStageJson struct {
	AccountId       string `json:"accountId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
	Audience        string `json:"audience"`
}

// awsOidcIdentityConfigureJson carries the (empty) payload sent by toJson for
// the verify/configure calls issued by UpsertFromStage; state is filled in by
// the backend from the stage step.
type awsOidcIdentityConfigureJson struct {
	State string `json:"state"`
}

func (r *AwsOidcIdentity) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_oidc_identity"
}

func (r *AwsOidcIdentity) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Allows P0 to grant and revoke AWS access for OIDC-authenticated agents (e.g. CI/CD pipelines), via
an AWS IAM identity provider federated on your OIDC provider. Requires the AWS integration (` + "`p0_aws_iam_write`" + `)
to be installed for the same account.

P0 uses ` + "`oidc_provider_url`" + ` and ` + "`audience`" + ` to generate AWS CLI/Terraform instructions for
registering the identity provider; it does not create the AWS-side identity provider on your behalf.`,
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

func (r *AwsOidcIdentity) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AwsOidcIdentity) getId(data any) *string {
	model, ok := data.(*awsOidcIdentityModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsOidcIdentity) getItemJson(json any) any {
	api, ok := json.(*awsOidcIdentityApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *AwsOidcIdentity) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*awsOidcIdentityJson)
	if !ok {
		return nil
	}
	return &awsOidcIdentityModel{
		Id:              id,
		AccountId:       json.AccountId,
		AwsPartition:    types.StringPointerValue(json.AwsPartition),
		OidcProviderUrl: json.OidcProviderUrl,
		Audience:        json.Audience,
	}
}

func (r *AwsOidcIdentity) toJson(data any) any {
	_, ok := data.(*awsOidcIdentityModel)
	if !ok {
		return nil
	}
	return &awsOidcIdentityConfigureJson{
		State: common.Config,
	}
}

func (r *AwsOidcIdentity) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsOidcIdentityModel{})

	var inputData awsOidcIdentityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &awsOidcIdentityStageJson{
		AccountId:       inputData.AccountId,
		OidcProviderUrl: inputData.OidcProviderUrl,
		Audience:        inputData.Audience,
	})
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsOidcIdentityModel{})
	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsOidcIdentityModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *AwsOidcIdentity) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
