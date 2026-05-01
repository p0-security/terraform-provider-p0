package installpostgres

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

var _ resource.Resource = &PostgresIamWriteStaged{}
var _ resource.ResourceWithImportState = &PostgresIamWriteStaged{}
var _ resource.ResourceWithConfigure = &PostgresIamWriteStaged{}

func NewPostgresIamWriteStaged() resource.Resource {
	return &PostgresIamWriteStaged{}
}

type PostgresIamWriteStaged struct {
	installer *common.Install
}

type postgresAwsConnectorHostingJson struct {
	Type         string  `json:"type" tfsdk:"type"`
	InstanceArn  string  `json:"instanceArn" tfsdk:"instance_arn"`
	ConnectorArn *string `json:"connectorArn" tfsdk:"connector_arn"`
	VpcId        string  `json:"vpcId" tfsdk:"vpc_id"`
}

type postgresAwsConnectorHostingModel struct {
	Type         string       `json:"type" tfsdk:"type"`
	InstanceArn  string       `json:"instanceArn" tfsdk:"instance_arn"`
	ConnectorArn types.String `json:"connectorArn" tfsdk:"connector_arn"`
	VpcId        string       `json:"vpcId" tfsdk:"vpc_id"`
}

type postgresIamWriteStagedJson struct {
	State   *string                          `json:"state"`
	Hosting *postgresAwsConnectorHostingJson `json:"hosting"`
}

type postgresIamWriteStagedApi struct {
	Item *postgresIamWriteStagedJson `json:"item"`
}

type postgresIamWriteStagedModel struct {
	Id      types.String                      `tfsdk:"id"`
	Hosting *postgresAwsConnectorHostingModel `tfsdk:"hosting"`
	State   types.String                      `tfsdk:"state"`
}

func (r *PostgresIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_staged"
}

func (r *PostgresIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged PostgreSQL installation. Staged resources generate the infrastructure configuration needed to deploy P0's PostgreSQL connector.

**Important:** If using RDS hosting, you must first install the p0_aws_rds resource for the instance's VPC.

Use the read-only attributes defined on this resource to get the shell commands or Terraform configuration needed to create the P0 connector infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A unique identifier for this PostgreSQL installation (can be any string, e.g., "production-db" or "staging-postgres")`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `How this instance (or cluster) is hosted`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: `The hosting environment`,
						Validators: []validator.String{
							stringvalidator.OneOf("aws-rds", "Hosting must be 'aws-rds'"),
						},
					},
					"connector_arn": schema.StringAttribute{
						MarkdownDescription: `The AWS Lambda connector ARN`,
						Computed:            true,
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
				},
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *PostgresIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  PostgresKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *PostgresIamWriteStaged) getId(data any) *string {
	model, ok := data.(*postgresIamWriteStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *PostgresIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*postgresIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *PostgresIamWriteStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := postgresIamWriteStagedModel{}

	jsonv, ok := json.(*postgresIamWriteStagedJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)

	if jsonv.State != nil {
		data.State = types.StringValue(*jsonv.State)
	}

	if jsonv.Hosting != nil {
		data.Hosting = &postgresAwsConnectorHostingModel{
			Type:         jsonv.Hosting.Type,
			ConnectorArn: types.StringPointerValue(jsonv.Hosting.ConnectorArn),
			InstanceArn:  jsonv.Hosting.InstanceArn,
			VpcId:        jsonv.Hosting.VpcId,
		}
	}

	return &data
}

func (r *PostgresIamWriteStaged) toJson(data any) any {
	json := postgresIamWriteStagedJson{}

	datav, ok := data.(*postgresIamWriteStagedModel)
	if !ok {
		return nil
	}

	json.Hosting = &postgresAwsConnectorHostingJson{
		Type:         datav.Hosting.Type,
		ConnectorArn: datav.Hosting.ConnectorArn.ValueStringPointer(),
		InstanceArn:  datav.Hosting.InstanceArn,
		VpcId:        datav.Hosting.VpcId,
	}

	return &json
}

func (r *PostgresIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json postgresIamWriteStagedApi
	var data postgresIamWriteStagedModel

	var inputData postgresIamWriteStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson := r.toJson(&inputData)

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *PostgresIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json postgresIamWriteStagedApi
	var data postgresIamWriteStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *PostgresIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json postgresIamWriteStagedApi
	var data postgresIamWriteStagedModel

	var inputData postgresIamWriteStagedModel
	req.Plan.Get(ctx, &inputData)
	inputJson := r.toJson(&inputData)

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *PostgresIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data postgresIamWriteStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *PostgresIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
