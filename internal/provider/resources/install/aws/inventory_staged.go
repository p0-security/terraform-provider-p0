// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

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
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsInventoryStaged{}
var _ resource.ResourceWithImportState = &AwsInventoryStaged{}
var _ resource.ResourceWithConfigure = &AwsInventoryStaged{}

func NewAwsInventoryStaged() resource.Resource {
	return &AwsInventoryStaged{}
}

type AwsInventoryStaged struct {
	installer *common.Install
}

type awsInventoryStagedApi struct {
	Item struct {
		Label        *string       `json:"label"`
		State        *string       `json:"state"`
		AwsPartition *AwsPartition `json:"awsPartition"`
	} `json:"item"`
	Metadata struct {
		InlinePolicy     string `json:"inlinePolicy"`
		InlinePolicyName string `json:"inlinePolicyName"`
		RoleName         string `json:"roleName"`
		ServiceAccountId string `json:"serviceAccountId"`
		TrustPolicy      string `json:"trustPolicy"`
	} `json:"metadata"`
}

type awsInventoryStagedModel struct {
	Id               string       `tfsdk:"id"`
	Partition        types.String `tfsdk:"partition"`
	Label            types.String `tfsdk:"label"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Role             types.Object `tfsdk:"role"`
}

func (r *AwsInventoryStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_inventory_staged"
}

func (r *AwsInventoryStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS resource inventory installation. Staged resources are used to generate AWS trust policies and role configuration.

**Important** Before using this resource, please read the instructions for the p0_aws_inventory resource. Create the AWS role using the attributes from this resource, then create the p0_aws_inventory resource with depends_on = [aws_iam_role.p0_inventory].`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
			},
			"partition": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("aws"),
				MarkdownDescription: `The AWS partition (aws or aws-us-gov). Defaults to aws if not specified.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsPartitionRegex, "AWS partition must be one of: aws, aws-us-gov."),
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

func (r *AwsInventoryStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  Aws,
		Component:    installresources.Inventory,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsInventoryStaged) getId(data any) *string {
	model, ok := data.(*awsInventoryStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsInventoryStaged) getItemJson(json any) any {
	inner, ok := json.(*awsInventoryStagedApi)
	if !ok {
		return nil
	}
	return inner
}

func (r *AwsInventoryStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsInventoryStagedModel{}

	jsonv, ok := json.(*awsInventoryStagedApi)
	if !ok {
		return nil
	}

	data.Id = id
	if jsonv.Item.Label != nil {
		data.Label = types.StringValue(*jsonv.Item.Label)
	}

	if jsonv.Item.AwsPartition != nil && jsonv.Item.AwsPartition.Type != nil {
		data.Partition = types.StringValue(*jsonv.Item.AwsPartition.Type)
	} else {
		data.Partition = types.StringValue("aws")
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

func (r *AwsInventoryStaged) toJson(data any) any {
	json := awsInventoryStagedApi{}

	datav, ok := data.(*awsInventoryStagedModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Item.Label = &label
	}

	if !datav.Partition.IsNull() && !datav.Partition.IsUnknown() {
		partition := datav.Partition.ValueString()
		json.Item.AwsPartition = &AwsPartition{Type: &partition}
	}

	return &json
}

func (r *AwsInventoryStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsInventoryStagedApi
	var data awsInventoryStagedModel

	var inputData awsInventoryStagedModel
	req.Config.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsInventoryStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsInventoryStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsInventoryStagedApi
	var data awsInventoryStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsInventoryStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsInventoryStagedApi
	var data awsInventoryStagedModel

	var inputData awsInventoryStagedModel
	req.Config.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsInventoryStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsInventoryStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsInventoryStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsInventoryStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
