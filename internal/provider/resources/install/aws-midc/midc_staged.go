// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installawsmidc

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
var _ resource.Resource = &AwsMidcStaged{}
var _ resource.ResourceWithImportState = &AwsMidcStaged{}
var _ resource.ResourceWithConfigure = &AwsMidcStaged{}

func NewAwsMidcStaged() resource.Resource {
	return &AwsMidcStaged{}
}

type AwsMidcStaged struct {
	installer *common.Install
}

type awsMidcStagedApi struct {
	Item struct {
		Label        *string                  `json:"label"`
		State        *string                  `json:"state"`
		AwsPartition *installaws.AwsPartition `json:"awsPartition"`
		IdcRegion    *string                  `json:"idcRegion"`
	} `json:"item"`
	Metadata struct {
		InlinePolicy     string `json:"inlinePolicy"`
		InlinePolicyName string `json:"inlinePolicyName"`
		RoleName         string `json:"roleName"`
		ServiceAccountId string `json:"serviceAccountId"`
		TrustPolicy      string `json:"trustPolicy"`
	} `json:"metadata"`
}

type awsMidcStagedModel struct {
	Id               string       `tfsdk:"id"`
	Partition        types.String `tfsdk:"partition"`
	IdcRegion        types.String `tfsdk:"idc_region"`
	Label            types.String `tfsdk:"label"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Role             types.Object `tfsdk:"role"`
}

func (r *AwsMidcStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_midc_staged"
}

func (r *AwsMidcStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS Identity Center (merged permission set) installation. Staged resources are used to generate AWS trust policies.

**Important**: Before using this resource, please read the instructions for the 'aws_midc' resource.
`,
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
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: installaws.AwsLabelMarkdownDescription,
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

func (r *AwsMidcStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AwsMidcStaged) getId(data any) *string {
	model, ok := data.(*awsMidcStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsMidcStaged) getItemJson(json any) any {
	inner, ok := json.(*awsMidcStagedApi)
	if !ok {
		return nil
	}
	return inner
}

func (r *AwsMidcStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsMidcStagedModel{}

	jsonv, ok := json.(*awsMidcStagedApi)
	if !ok {
		return nil
	}

	data.Id = id
	if jsonv.Item.Label != nil {
		data.Label = types.StringValue(*jsonv.Item.Label)
	}

	if jsonv.Item.AwsPartition != nil && jsonv.Item.AwsPartition.Type != nil {
		data.Partition = types.StringValue(*jsonv.Item.AwsPartition.Type)
	}

	if jsonv.Item.IdcRegion != nil {
		data.IdcRegion = types.StringValue(*jsonv.Item.IdcRegion)
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
		diags.Append(objErr...)
		return nil
	}
	data.Role = role

	return &data
}

func (r *AwsMidcStaged) toJson(data any) any {
	json := awsMidcStagedApi{}

	datav, ok := data.(*awsMidcStagedModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Item.Label = &label
	}

	if !datav.Partition.IsNull() && !datav.Partition.IsUnknown() {
		partition := datav.Partition.ValueString()
		json.Item.AwsPartition = &installaws.AwsPartition{Type: &partition}
	}

	if !datav.IdcRegion.IsNull() && !datav.IdcRegion.IsUnknown() {
		idcRegion := datav.IdcRegion.ValueString()
		json.Item.IdcRegion = &idcRegion
	}

	// can omit state here as it's filled by the backend
	return &json
}

func (r *AwsMidcStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsMidcStagedApi
	var data awsMidcStagedModel

	// Read from the plan, not the config: schema defaults (e.g. partition) are
	// only applied to the plan, and the backend rejects a null awsPartition.
	var inputData awsMidcStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsMidcStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsMidcStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsMidcStagedApi
	var data awsMidcStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsMidcStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsMidcStagedApi
	var data awsMidcStagedModel

	var inputData awsMidcStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsMidcStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsMidcStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsMidcStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsMidcStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
