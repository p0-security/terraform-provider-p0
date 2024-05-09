// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsIamWriteStaged{}
var _ resource.ResourceWithImportState = &AwsIamWriteStaged{}
var _ resource.ResourceWithConfigure = &AwsIamWriteStaged{}

func NewIamWriteStagedAws() resource.Resource {
	return &AwsIamWriteStaged{}
}

type AwsIamWriteStaged struct {
	installer *installresources.Install
}

type awsIamWriteStagedApi struct {
	Item struct {
		Label *string `json:"label"`
		State *string `json:"state"`
	} `json:"item"`
	Metadata struct {
		InlinePolicy     string `json:"inlinePolicy"`
		InlinePolicyName string `json:"inlinePolicyName"`
		RoleName         string `json:"roleName"`
		ServiceAccountId string `json:"serviceAccountId"`
		TrustPolicy      string `json:"trustPolicy"`
	} `json:"metadata"`
}

type awsIamWriteStagedModel struct {
	Id               string       `tfsdk:"id"`
	Label            types.String `tfsdk:"label"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Role             types.Object `tfsdk:"role"`
}

func (r *AwsIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_iam_write_staged"
}

func (r *AwsIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS IAM-management installation. Staged resources are used to generate AWS trust policies.

**Important** Before using this resource, please read the instructions for the 'aws_iam_write' resource.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: AwsLabelMarkdownDescription,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The audience ID of the service account to include in this AWS account's P0 role trust policies`,
			},
			"role": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Describes the AWS role that this P0 component uses to access AWS account infrastructure`,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The AWS role name`,
					},
					"inline_policy": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The inline policy that should be attached to the AWS role`,
					},
					"inline_policy_name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The name of the inline policy that should be attached to the AWS role`,
					},
					"trust_policy": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The trust policy that should be attached to the AWS role`,
					},
				},
			},
		},
	}
}

func (r *AwsIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		Integration:  Aws,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsIamWriteStaged) getId(data any) *string {
	model, ok := data.(*awsIamWriteStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*awsIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner
}

func (r *AwsIamWriteStaged) fromJson(id string, json any) any {
	data := awsIamWriteStagedModel{}

	jsonv, ok := json.(*awsIamWriteStagedApi)
	if !ok {
		return nil
	}

	data.Id = id
	if jsonv.Item.Label != nil {
		data.Label = types.StringValue(*jsonv.Item.Label)
	}
	data.ServiceAccountId = types.StringValue(jsonv.Metadata.ServiceAccountId)

	role, objErr := types.ObjectValue(
		map[string]attr.Type{
			"inline_policy":      types.StringType,
			"inline_policy_name": types.StringType,
			"name":               types.StringType,
			"trust_policy":       types.StringType,
		},
		map[string]attr.Value{
			"inline_policy":      types.StringValue(jsonv.Metadata.InlinePolicy),
			"inline_policy_name": types.StringValue(jsonv.Metadata.InlinePolicyName),
			"name":               types.StringValue(jsonv.Metadata.RoleName),
			"trust_policy":       types.StringValue(jsonv.Metadata.TrustPolicy),
		},
	)
	if objErr.HasError() {
		return nil
	}
	data.Role = role

	return &data
}

func (r *AwsIamWriteStaged) toJson(data any) any {
	json := awsIamWriteStagedApi{}

	datav, ok := data.(*awsIamWriteStagedModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Item.Label = &label
	}

	// can omit state here as it's filled by the backend
	return &json
}

func (r *AwsIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsIamWriteStagedApi
	var data awsIamWriteStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsIamWriteStagedApi
	var data awsIamWriteStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsIamWriteStagedApi
	var data awsIamWriteStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsIamWriteStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
