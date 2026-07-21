// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installawsmidc

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsMidc{}
var _ resource.ResourceWithImportState = &AwsMidc{}
var _ resource.ResourceWithConfigure = &AwsMidc{}

func NewAwsMidc() resource.Resource {
	return &AwsMidc{}
}

type AwsMidc struct {
	installer *common.Install
}

type awsMidcIdentityJson struct {
	Type *string `json:"type"`
}

type awsMidcJson struct {
	Label           *string                  `json:"label"`
	State           string                   `json:"state"`
	AwsPartition    *installaws.AwsPartition `json:"awsPartition"`
	IdcRegion       *string                  `json:"idcRegion"`
	Identity        *awsMidcIdentityJson     `json:"identity"`
	IdcArn          *string                  `json:"idcArn,omitempty"`
	IdentityStoreId *string                  `json:"identityStoreId,omitempty"`
}

type awsMidcApi struct {
	Item *awsMidcJson `json:"item"`
}

type awsMidcModel struct {
	Id              string       `tfsdk:"id"`
	Partition       types.String `tfsdk:"partition"`
	IdcRegion       types.String `tfsdk:"idc_region"`
	Identity        types.String `tfsdk:"identity"`
	Label           types.String `tfsdk:"label"`
	State           types.String `tfsdk:"state"`
	IdcArn          types.String `tfsdk:"idc_arn"`
	IdentityStoreId types.String `tfsdk:"identity_store_id"`
}

func (r *AwsMidc) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_midc"
}

func (r *AwsMidc) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Note that the TF doc generator clobbers _most_ underscores :(
		MarkdownDescription: `An AWS Identity Center (merged permission set) installation. Allows P0 to grant and revoke AWS access
via Identity Center permission sets and resource policies. Install this in the AWS account that contains your
Identity Center instance; individual AWS accounts may then use it for just-in-time access by configuring the
'merged-idc' login type on their 'aws_iam_write' resource.

**Important**: This resource should be used together with the 'aws_midc_staged' resource, with a dependency chain
requiring this resource to be updated after the 'aws_midc_staged' resource.

P0 recommends you use these resources according to the following pattern:

` + "```terraform" + // Go does not support escaping backticks in literals, see https://github.com/golang/go/issues/32590 and its many friends
			`
resource "p0_aws_midc_staged" "staged_account" {
  id         = ...
  idc_region = ...
}

resource "aws_iam_role" "p0_midc_manager" {
  name               = p0_aws_midc_staged.staged_account.role.name
  assume_role_policy = p0_aws_midc_staged.staged_account.role.trust_policy
}

resource "aws_iam_role_policy" "p0_midc_manager" {
  name   = p0_aws_midc_staged.staged_account.role.inline_policy_name
  role   = aws_iam_role.p0_midc_manager.name
  policy = p0_aws_midc_staged.staged_account.role.inline_policy
}

resource "p0_aws_midc" "installed_account" {
  id         = p0_aws_midc_staged.staged_account.id
  idc_region = p0_aws_midc_staged.staged_account.idc_region
  partition  = p0_aws_midc_staged.staged_account.partition
  depends_on = [aws_iam_role_policy.p0_midc_manager]
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID of the account that contains the Identity Center instance`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
			},
			"partition": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("aws"),
				MarkdownDescription: `The AWS partition (aws or aws-us-gov). Defaults to aws if not specified.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsPartitionRegex, "AWS partition must be one of: aws, aws-us-gov."),
				},
			},
			"idc_region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS region where the Identity Center instance is installed (e.g. us-east-1)`,
			},
			"identity": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("user"),
				MarkdownDescription: `How users are identified in AWS Identity Center; one of:
    - 'user': Username is user's email (default)
    - 'email': User's IDC email is user's email`,
				Validators: []validator.String{
					stringvalidator.OneOf("user", "email"),
				},
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: installaws.AwsLabelMarkdownDescription,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
			"idc_arn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The unique ARN of the Identity Center instance`,
			},
			"identity_store_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `ID of the identity store that is connected to the Identity Center instance`,
			},
		},
	}
}

func (r *AwsMidc) getId(data any) *string {
	model, ok := data.(*awsMidcModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsMidc) getItemJson(json any) any {
	inner, ok := json.(*awsMidcApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsMidc) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsMidcModel{}
	jsonv, ok := json.(*awsMidcJson)
	if !ok {
		return nil
	}

	data.Id = id

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.Partition = types.StringNull()
	if jsonv.AwsPartition != nil && jsonv.AwsPartition.Type != nil {
		data.Partition = types.StringValue(*jsonv.AwsPartition.Type)
	}

	data.IdcRegion = types.StringNull()
	if jsonv.IdcRegion != nil {
		data.IdcRegion = types.StringValue(*jsonv.IdcRegion)
	}

	data.Identity = types.StringNull()
	if jsonv.Identity != nil && jsonv.Identity.Type != nil {
		data.Identity = types.StringValue(*jsonv.Identity.Type)
	}

	data.IdcArn = types.StringNull()
	if jsonv.IdcArn != nil {
		data.IdcArn = types.StringValue(*jsonv.IdcArn)
	}

	data.IdentityStoreId = types.StringNull()
	if jsonv.IdentityStoreId != nil {
		data.IdentityStoreId = types.StringValue(*jsonv.IdentityStoreId)
	}

	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *AwsMidc) toJson(data any) any {
	json := awsMidcJson{}

	datav, ok := data.(*awsMidcModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	if !datav.Partition.IsNull() && !datav.Partition.IsUnknown() {
		partition := datav.Partition.ValueString()
		json.AwsPartition = &installaws.AwsPartition{Type: &partition}
	}

	if !datav.IdcRegion.IsNull() && !datav.IdcRegion.IsUnknown() {
		idcRegion := datav.IdcRegion.ValueString()
		json.IdcRegion = &idcRegion
	}

	if !datav.Identity.IsNull() && !datav.Identity.IsUnknown() {
		identity := datav.Identity.ValueString()
		json.Identity = &awsMidcIdentityJson{Type: &identity}
	}

	// can omit state here as it's filled by the backend
	return &json
}

func (r *AwsMidc) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AwsMidcKey,
		Component:    installresources.Identity,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsMidc) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsMidcApi
	var data awsMidcModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsMidc) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsMidcApi
	var data awsMidcModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsMidc) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsMidcApi
	var data awsMidcModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsMidc) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsMidcModel
	// Return the item to the "stage" state; the companion staged resource's
	// Delete removes it from P0.
	r.installer.Rollback(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsMidc) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
