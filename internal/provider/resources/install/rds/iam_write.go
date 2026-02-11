package installrds

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &rdsIamWrite{}
var _ resource.ResourceWithConfigure = &rdsIamWrite{}
var _ resource.ResourceWithImportState = &rdsIamWrite{}

type rdsIamWrite struct {
	installer *common.Install
}

type rdsIamWriteModel struct {
	Id        types.String `tfsdk:"id" json:"id,omitempty"`
	AccountId types.String `tfsdk:"account_id" json:"accountId,omitempty"`
	Region    types.String `tfsdk:"region" json:"region,omitempty"`
	State     types.String `tfsdk:"state" json:"state,omitempty"`
	Label     types.String `tfsdk:"label" json:"label,omitempty"`
}

type rdsIamWriteJson struct {
	AccountId string  `json:"accountId"`
	Region    string  `json:"region"`
	State     string  `json:"state"`
	Label     *string `json:"label,omitempty"`
}

type rdsIamWriteApi struct {
	Item *rdsIamWriteJson `json:"item"`
}

func NewRdsIamWrite() resource.Resource {
	return &rdsIamWrite{}
}

// Metadata implements resource.ResourceWithImportState.
func (*rdsIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_aws_rds"
}

// Schema implements resource.ResourceWithImportState.
func (*rdsIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS RDS Installation.

Installing RDS allows you to manage access to your RDS database instances using IAM authentication.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: `The VPC ID for the RDS installation`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsVpcIdRegex, "VPC IDs should be in the format: vpc-xxxxxxxx"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID containing the RDS instances`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should be numeric"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: `The AWS region where the RDS instances are located`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsRegionRegex, "AWS region should be in the format: us-east-1"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: AwsLabelMarkdownDescription,
				Computed:            true,
				Optional:            true,
			},
		},
	}
}

func (r *rdsIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  RdsKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *rdsIamWrite) getId(data any) *string {
	model, ok := data.(*rdsIamWriteModel)
	if !ok {
		return nil
	}

	str := model.Id.ValueString()
	return &str
}

func (r *rdsIamWrite) getItemJson(json any) any {
	inner, ok := json.(*rdsIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *rdsIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := rdsIamWriteModel{}
	jsonv, ok := json.(*rdsIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.AccountId = types.StringValue(jsonv.AccountId)
	data.Region = types.StringValue(jsonv.Region)
	data.State = types.StringValue(jsonv.State)

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		label := types.StringValue(*jsonv.Label)
		data.Label = label
	}

	return &data
}

func (r *rdsIamWrite) toJson(data any) any {
	json := rdsIamWriteJson{}

	datav, ok := data.(*rdsIamWriteModel)
	if !ok {
		return nil
	}

	json.AccountId = datav.AccountId.ValueString()
	json.Region = datav.Region.ValueString()

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	// can omit state here as it's filled by the backend
	return &json
}

// Create implements resource.ResourceWithImportState.
func (s *rdsIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json rdsIamWriteApi
	var data rdsIamWriteModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)

	// Convert the model to JSON for the Stage call
	// This ensures fields marked with step: "new" are sent during the assemble step
	inputJson := s.toJson(&data)

	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *rdsIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &rdsIamWriteApi{}, &rdsIamWriteModel{})
}

func (s *rdsIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &rdsIamWriteModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *rdsIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &rdsIamWriteApi{}, &rdsIamWriteModel{})
}

func (s *rdsIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
