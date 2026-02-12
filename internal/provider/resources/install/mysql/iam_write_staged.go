package installmysql

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

var _ resource.Resource = &MysqlIamWriteStaged{}
var _ resource.ResourceWithImportState = &MysqlIamWriteStaged{}
var _ resource.ResourceWithConfigure = &MysqlIamWriteStaged{}

func NewMysqlIamWriteStaged() resource.Resource {
	return &MysqlIamWriteStaged{}
}

type MysqlIamWriteStaged struct {
	installer *common.Install
}

type mysqlIamWriteStagedApi struct {
	Item struct {
		State   *string `json:"state"`
		Hosting *struct {
			Type         string `json:"type"`
			InstanceArn  string `json:"instanceArn"`
			VpcId        string `json:"vpcId"`
			ConnectorArn string `json:"connectorArn"`
		} `json:"hosting"`
	} `json:"item"`
}

type mysqlIamWriteStagedModel struct {
	Id          types.String `tfsdk:"id"`
	InstanceArn types.String `tfsdk:"instance_arn"`
	VpcId       types.String `tfsdk:"vpc_id"`
	Region      types.String `tfsdk:"region"`
	AccountId   types.String `tfsdk:"account_id"`
	State       types.String `tfsdk:"state"`
}

func (r *MysqlIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mysql_staged"
}

func (r *MysqlIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged MySQL installation for AWS RDS. Staged resources generate the infrastructure configuration needed to deploy the Lambda connector.

**Important:** Before using this resource, you must first install the p0_aws_rds resource for the VPC.

Use the read-only attributes defined on this resource to get the shell commands or Terraform configuration needed to create the Lambda connector infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A unique identifier for this MySQL installation (can be any string, e.g., "production-db" or "staging-mysql")`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"instance_arn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS RDS instance ARN`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsRdsArnRegex, "Must be a valid AWS RDS instance ARN"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS VPC ID where the RDS instance is located (must reference an existing aws-rds integration)`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsVpcIdRegex, "Must be a valid AWS VPC ID"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The AWS region (computed from RDS perimeter configuration)`,
			},
			"account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The AWS account ID (computed from RDS perimeter configuration)`,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *MysqlIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  MysqlKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *MysqlIamWriteStaged) getId(data any) *string {
	model, ok := data.(*mysqlIamWriteStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *MysqlIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*mysqlIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner
}

func (r *MysqlIamWriteStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := mysqlIamWriteStagedModel{}

	jsonv, ok := json.(*mysqlIamWriteStagedApi)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)

	if jsonv.Item.State != nil {
		data.State = types.StringValue(*jsonv.Item.State)
	}

	if jsonv.Item.Hosting != nil {
		data.InstanceArn = types.StringValue(jsonv.Item.Hosting.InstanceArn)
		data.VpcId = types.StringValue(jsonv.Item.Hosting.VpcId)

		region, accountId := parseRdsArn(jsonv.Item.Hosting.InstanceArn)
		data.Region = types.StringValue(region)
		data.AccountId = types.StringValue(accountId)
	}

	return &data
}

func (r *MysqlIamWriteStaged) toJson(data any) any {
	datav, ok := data.(*mysqlIamWriteStagedModel)
	if !ok {
		return nil
	}

	return &struct {
		Hosting struct {
			Type        string `json:"type"`
			InstanceArn string `json:"instanceArn"`
			VpcId       string `json:"vpcId"`
		} `json:"hosting"`
	}{
		Hosting: struct {
			Type        string `json:"type"`
			InstanceArn string `json:"instanceArn"`
			VpcId       string `json:"vpcId"`
		}{
			Type:        "aws-rds",
			InstanceArn: datav.InstanceArn.ValueString(),
			VpcId:       datav.VpcId.ValueString(),
		},
	}
}

func (r *MysqlIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json mysqlIamWriteStagedApi
	var data mysqlIamWriteStagedModel

	var inputData mysqlIamWriteStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson := r.toJson(&inputData)

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *MysqlIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json mysqlIamWriteStagedApi
	var data mysqlIamWriteStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *MysqlIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json mysqlIamWriteStagedApi
	var data mysqlIamWriteStagedModel

	var inputData mysqlIamWriteStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson := r.toJson(&inputData)

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *MysqlIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data mysqlIamWriteStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *MysqlIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
