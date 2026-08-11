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
	AccountId       types.String `tfsdk:"account_id"`
	AwsPartition    types.String `tfsdk:"aws_partition"`
	OidcProviderUrl types.String `tfsdk:"oidc_provider_url"`
	Audience        types.String `tfsdk:"audience"`
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
// installation". None of this component's fields are mutable after creation
// — that's why they're only settable on p0_aws_oidc_identity_staged.
type awsOidcIdentityStageJson struct {
	AccountId       string `json:"accountId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
	Audience        string `json:"audience"`
}

// awsOidcIdentityConfigureJson carries the payload sent by toJson for the
// verify/configure calls issued by UpsertFromStage (and the PUT issued by
// Rollback on delete). These fields are resent here even though they're
// `step: "new"` — the same, unchanged values the user already staged —
// because `accountId` is read by the `awsPartition` field's assembler from
// the raw unmerged request body, so omitting it would silently clear
// `awsPartition` rather than just failing to change it; `oidcProviderUrl`
// and `audience` are resent alongside it for consistency. This mirrors
// p0_okta_directory_listing's final resource, which resends its own
// immutable fields the same way.
type awsOidcIdentityConfigureJson struct {
	AccountId       string `json:"accountId"`
	OidcProviderUrl string `json:"oidcProviderUrl"`
	Audience        string `json:"audience"`
	State           string `json:"state"`
}

func (r *AwsOidcIdentity) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_oidc_identity"
}

func (r *AwsOidcIdentity) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final installation of an AWS OIDC identity federation, allowing P0 to grant and revoke AWS
access for OIDC-authenticated agents (e.g. CI/CD pipelines).

To use this resource, you must also:
- install the ` + "`p0_aws_oidc_identity_staged`" + ` resource,
- create an AWS IAM OIDC identity provider trusting your OIDC provider (` + "`aws_iam_openid_connect_provider`" + `), and
- create the IAM role(s) P0 federates into, with a trust policy scoped to that provider and the ` + "`audience`" + `
  from the staged resource.

Requires the AWS integration (` + "`p0_aws_iam_write`" + `) to be installed for the same account. See the example
usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `p0_aws_oidc_identity_staged` resource being finalized",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS account ID to federate access into. Must match `account_id` on the `p0_aws_oidc_identity_staged` resource.",
			},
			"aws_partition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The AWS partition (e.g. `aws`, `aws-us-gov`) that the account belongs to",
			},
			"oidc_provider_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Issuer URL of your OIDC provider. Must match `oidc_provider_url` on the `p0_aws_oidc_identity_staged` resource.",
			},
			"audience": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `aud` claim the identity will send to AWS when calling APIs. Must match `audience` on the `p0_aws_oidc_identity_staged` resource.",
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
		AccountId:       types.StringValue(json.AccountId),
		AwsPartition:    types.StringPointerValue(json.AwsPartition),
		OidcProviderUrl: types.StringValue(json.OidcProviderUrl),
		Audience:        types.StringValue(json.Audience),
	}
}

func (r *AwsOidcIdentity) toJson(data any) any {
	model, ok := data.(*awsOidcIdentityModel)
	if !ok {
		return nil
	}
	return &awsOidcIdentityConfigureJson{
		AccountId:       model.AccountId.ValueString(),
		OidcProviderUrl: model.OidcProviderUrl.ValueString(),
		Audience:        model.Audience.ValueString(),
		State:           common.Config,
	}
}

func (r *AwsOidcIdentity) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsOidcIdentityApi
	var data awsOidcIdentityModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsOidcIdentity) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsOidcIdentityModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *AwsOidcIdentity) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
