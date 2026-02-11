// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsInventory{}
var _ resource.ResourceWithImportState = &AwsInventory{}
var _ resource.ResourceWithConfigure = &AwsInventory{}

func NewAwsInventory() resource.Resource {
	return &AwsInventory{}
}

type AwsInventory struct {
	installer *common.Install
}

type awsInventoryModel struct {
	Id        string                `tfsdk:"id"`
	Partition basetypes.StringValue `tfsdk:"partition"`
	Label     basetypes.StringValue `tfsdk:"label"`
	State     basetypes.StringValue `tfsdk:"state"`
}

type awsInventoryJson struct {
	Label        *string       `json:"label"`
	State        string        `json:"state"`
	AwsPartition *AwsPartition `json:"awsPartition"`
}

type awsInventoryApi struct {
	Item *awsInventoryJson `json:"item"`
}

func (r *AwsInventory) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_inventory"
}

func (r *AwsInventory) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS resource inventory installation.

Allows P0 to inventory your AWS resources. Required for resource-level just-in-time access.

**Important**: This resource should be used together with the p0_aws_inventory_staged resource, with a dependency chain
requiring this resource to be updated after the p0_aws_inventory_staged resource.

Create the AWS role using the attributes from p0_aws_inventory_staged, then create this resource with depends_on = [aws_iam_role.p0_inventory].`,
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
				Optional:            true,
				Computed:            true,
				MarkdownDescription: AwsLabelMarkdownDescription,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
		},
	}
}

func (r *AwsInventory) getId(data any) *string {
	model, ok := data.(*awsInventoryModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsInventory) getItemJson(json any) any {
	inner, ok := json.(*awsInventoryApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsInventory) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsInventoryModel{}
	jsonv, ok := json.(*awsInventoryJson)
	if !ok {
		return nil
	}

	data.Id = id

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	if jsonv.AwsPartition != nil && jsonv.AwsPartition.Type != nil {
		data.Partition = types.StringValue(*jsonv.AwsPartition.Type)
	} else {
		data.Partition = types.StringValue("aws")
	}

	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *AwsInventory) toJson(data any) any {
	json := awsInventoryJson{}

	datav, ok := data.(*awsInventoryModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	if !datav.Partition.IsNull() && !datav.Partition.IsUnknown() {
		partition := datav.Partition.ValueString()
		json.AwsPartition = &AwsPartition{Type: &partition}
	}

	// state is filled by the backend
	return &json
}

func (r *AwsInventory) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AwsInventory) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsInventoryApi
	var data awsInventoryModel
	req.Config.Get(ctx, &data)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsInventory) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsInventoryApi
	var data awsInventoryModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsInventory) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsInventoryApi
	var data awsInventoryModel
	req.Config.Get(ctx, &data)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsInventory) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsInventoryModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsInventory) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
