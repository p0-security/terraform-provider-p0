// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &StagedAws{}
var _ resource.ResourceWithImportState = &StagedAws{}
var _ resource.ResourceWithConfigure = &StagedAws{}

func NewAwsIamWrite() resource.Resource {
	return &AwsIamWrite{}
}

type AwsIamWrite struct {
	installer *installresources.Install
}

type awsIamWriteLoginIdentityModel struct {
	Type    string  `json:"type" tfsdk:"type"`
	TagName *string `json:"tagName" tfsdk:"tag_name"`
}

type awsIamWriteLoginProviderMethodModel struct {
	Type         string `json:"type" tfsdk:"type"`
	AccountCount *struct {
		Type   string  `json:"type" tfsdk:"type"`
		Parent *string `json:"parent" tfsdk:"parent"`
	} `json:"accountCount" tfsdk:"account_count"`
}

type awsIamWriteLoginProviderModel struct {
	Type             string                               `json:"type" tfsdk:"type"`
	AppId            *string                              `json:"appId" tfsdk:"app_id"`
	IdentityProvider *string                              `json:"identityProvider" tfsdk:"identity_provider"`
	Method           *awsIamWriteLoginProviderMethodModel `json:"method" tfsdk:"method"`
}

type awsIamWriteLoginModel struct {
	Type     string                         `json:"type" tfsdk:"type"`
	Identity *awsIamWriteLoginIdentityModel `json:"identity" tfsdk:"identity"`
	Parent   *string                        `json:"parent" tfsdk:"parent"`
	Provider *awsIamWriteLoginProviderModel `json:"provider" tfsdk:"provider"`
}

type awsIamWriteModel struct {
	Id    string                 `tfsdk:"id"`
	Label basetypes.StringValue  `tfsdk:"label"`
	State basetypes.StringValue  `tfsdk:"state"`
	Login *awsIamWriteLoginModel `tfsdk:"login"`
}

type awsIamWriteJson struct {
	Label *string                `json:"label"`
	State string                 `json:"state"`
	Login *awsIamWriteLoginModel `json:"login"`
}

type awsIamWriteApi struct {
	Item *awsIamWriteJson `json:"item"`
}

func (r *AwsIamWrite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_iam_write"
}

func (r *AwsIamWrite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Note that the TF doc generator clobbers _most_ underscores :(
		MarkdownDescription: `An AWS installation.

**Important**: This resource should be used together with the 'aws_staged' resource, with a dependency chain
requiring this resource to be updated after the 'aws_staged' resource.

P0 recommends you use these resources according to the following pattern:

` + "```" + // Go does not support escaping backticks in literals, see https://github.com/golang/go/issues/32590 and its many friends
			`
resource "p0_aws_staged" "staged_account" {
  id         = ...
  components = ["iam-write"]
}

# See current P0 docs for the appropriate input in this block
resource "aws_iam_policy" "p0_iam_manager" {
  ...
}

resource "aws_iam_role" "p0_iam_manager" {
  name               = "P0RoleIamManager"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = "accounts.google.com"
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "accounts.google.com:aud" = "${p0_aws_staged.staged_account.service_account_id}"
          }
        }
      }
    ]
  })
  managed_policy_arns = [aws_iam_policy.p0_iam_manager.arn]
}

resource "p0_aws_iam_write" "installed_account" {
  id         = p0_aws_staged.staged_account.id
  depends_on = [aws_iam_role.p0_iam_manager]
  ...
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: `The AWS account's alias (if available)`,
			},
			"state": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: `This account's install progress in the P0 application:
		- 'stage': The account has been staged for installation
		- 'configure': The account is available to be added to P0, and may be configured
		- 'installed': The account is fully installed`,
			},
			"login": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `How users log in to this AWS account`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required: true,
						MarkdownDescription: `One of:
    - 'iam': Users log in as IAM users; 'identity' attribute is required
    - 'idc': Users log in via Identity Center (formerly 'SSO'); 'parent' attribute is required
    - 'federated': Users log in via a federated identity provider; 'provider' attribute is required`,
						Validators: []validator.String{
							stringvalidator.AnyWithAllWarnings(
								stringvalidator.All(
									stringvalidator.OneOf("iam"),
									stringvalidator.AlsoRequires(
										path.MatchRelative().AtParent().AtName("identity"),
									),
								),
								stringvalidator.All(
									stringvalidator.OneOf("idc"),
									stringvalidator.AlsoRequires(
										path.MatchRelative().AtParent().AtName("parent"),
									),
								),
								stringvalidator.All(
									stringvalidator.OneOf("federated"),
									stringvalidator.AlsoRequires(
										path.MatchRelative().AtParent().AtName("provider"),
									),
								),
							),
						},
					},
					"identity": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: `How user identities are mapped to AWS IAM users`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required: true,
								MarkdownDescription: `One of:
    - 'email': IAM user names are user email addresses
    - 'tag': User email addresses appear in IAM user tag; 'tag_name' is required`,
								Validators: []validator.String{
									stringvalidator.AnyWithAllWarnings(
										stringvalidator.All(
											stringvalidator.OneOf("email"),
											stringvalidator.ConflictsWith(
												path.MatchRelative().AtParent().AtName("tag_name")),
										),
										stringvalidator.All(
											stringvalidator.OneOf("tag"),
											stringvalidator.AlsoRequires(
												path.MatchRelative().AtParent().AtName("tag_name")),
										),
									),
								},
							},
							"tag_name": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: `The name of the AWS user tag that holds the user's email address`,
							},
						},
						Validators: []validator.Object{
							objectvalidator.ConflictsWith(
								path.MatchRelative().AtParent().AtName("provider"),
								path.MatchRelative().AtParent().AtName("parent"),
							),
						},
					},
					"parent": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `Identity Center parent account ID`,
						Validators: []validator.String{
							stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("identity")),
							stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("provider")),
							stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
						},
					},
					"provider": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: `Federated login provider details`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Optional:            true,
								Computed:            true,
								Default:             stringdefault.StaticString("okta"),
								MarkdownDescription: "Only 'okta' is supported at this time",
								Validators:          []validator.String{stringvalidator.OneOf("okta")},
							},
							"app_id": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Okta AWS federation app ID",
								Validators:          []validator.String{stringvalidator.RegexMatches(OktaAppIdRegex, "Okta app IDs should start with '0o'")},
							},
							"identity_provider": schema.StringAttribute{
								Required: true,
								MarkdownDescription: `AWS provider integration; this is the _name_ of the AWS integration that you use for federated login,
defined on the ["Identity providers" tab](https://console.aws.amazon.com/iam/home#/identity_providers) of your IAM dashboard`,
							},
							"method": schema.SingleNestedAttribute{
								Required:            true,
								MarkdownDescription: `The federation method used by your identity provider`,
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{
										Optional:            true,
										Computed:            true,
										Default:             stringdefault.StaticString("saml"),
										MarkdownDescription: `Only 'saml' is supported at this time`,
										Validators:          []validator.String{stringvalidator.OneOf("saml")},
									},
									"account_count": schema.SingleNestedAttribute{
										Required: true,
										MarkdownDescription: `Number of AWS accounts linked to the federation app:
    - 'single': One account only
    - 'multi': Multiple accounts, via a parent account`,
										Attributes: map[string]schema.Attribute{
											"type": schema.StringAttribute{
												Optional: true,
												Computed: true,
												Default:  stringdefault.StaticString("single"),
												Validators: []validator.String{
													stringvalidator.AnyWithAllWarnings(
														stringvalidator.All(
															stringvalidator.OneOf("single"),
															stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("parent")),
														),
														stringvalidator.All(
															stringvalidator.OneOf("multi"),
															stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("parent")),
														),
													),
												},
											},
											"parent": schema.StringAttribute{
												Optional:            true,
												MarkdownDescription: `The account ID of the federation app's parent AWS account`,
												Validators: []validator.String{
													stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
												},
											},
										},
									},
								},
							},
						},
						Validators: []validator.Object{
							objectvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("identity")),
							objectvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("parent")),
						},
					},
				},
			},
		},
	}
}

func (r *AwsIamWrite) getId(data any) *string {
	model, ok := data.(*awsIamWriteModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsIamWrite) getItemJson(json any) any {
	inner, ok := json.(*awsIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsIamWrite) fromJson(id string, json any) any {
	data := awsIamWriteModel{}
	jsonv, ok := json.(*awsIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = id
	data.Label = types.StringNull()
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.State = types.StringValue(jsonv.State)
	data.Login = jsonv.Login

	return &data
}

func (r *AwsIamWrite) toJson(data any) any {
	json := awsIamWriteJson{}

	datav, ok := data.(*awsIamWriteModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	// can omit state here as it's filled by the backend
	json.Login = datav.Login

	return &json
}

func (r *AwsIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		Component:    IamWrite,
		Integration:  Aws,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsIamWriteApi
	var data awsIamWriteModel
	r.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsIamWriteApi
	var data awsIamWriteModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsIamWriteApi
	var data awsIamWriteModel
	r.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsIamWriteModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
