package installfiletransfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

const awsPrefix = "aws:"

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &fileTransferIamWrite{}
var _ resource.ResourceWithConfigure = &fileTransferIamWrite{}
var _ resource.ResourceWithImportState = &fileTransferIamWrite{}

type fileTransferIamWrite struct {
	installer *common.Install
}

type fileTransferIamWriteModel struct {
	AccountId    types.String `tfsdk:"account_id"`
	BucketName   types.String `tfsdk:"bucket_name"`
	Region       types.String `tfsdk:"region"`
	AwsPartition types.String `tfsdk:"aws_partition"`
	Label        types.String `tfsdk:"label"`
	State        types.String `tfsdk:"state"`
}

type fileTransferIamWriteJson struct {
	BucketName   string  `json:"bucketName"`
	Region       string  `json:"region"`
	AwsPartition string  `json:"awsPartition"`
	State        *string `json:"state,omitempty"`
	Label        *string `json:"label,omitempty"`
}

type fileTransferIamWriteApi struct {
	Item *fileTransferIamWriteJson `json:"item"`
}

func NewFileTransferIamWrite() resource.Resource {
	return &fileTransferIamWrite{}
}

// Metadata implements resource.ResourceWithImportState.
func (*fileTransferIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_file_transfer"
}

// Schema implements resource.ResourceWithImportState.
func (*fileTransferIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A File Transfer installation for AWS.

Installing File Transfer allows P0 to broker temporary, audited file transfers to your AWS EC2 instances through a customer-owned S3 bucket.

**Prerequisite:** AWS SSH (` + "`p0_ssh_aws`" + `) must be installed for the same AWS account before file transfer can be requested.`,
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID. AWS SSH must already be installed for this account.`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should be numeric"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bucket_name": schema.StringAttribute{
				MarkdownDescription: `The name of the S3 bucket used to broker fast file transfers (without the ` + "`s3://`" + ` prefix)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(BucketNameRegex, "Must be a valid S3 bucket DNS name (lowercase letters, numbers, dots, and hyphens)"),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: `The AWS region of the S3 bucket (e.g. ` + "`us-east-1`" + `)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(RegionRegex, "AWS region should be in the format: us-east-1 or us-gov-east-1"),
				},
			},
			"aws_partition": schema.StringAttribute{
				MarkdownDescription: `The AWS partition the bucket resides in. Usually ` + "`aws`" + `; use ` + "`aws-us-gov`" + ` for GovCloud or ` + "`aws-cn`" + ` for China`,
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(DefaultPartition),
				Validators: []validator.String{
					stringvalidator.OneOf(Partitions...),
				},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: `The label for this installation (defaults to the AWS account ID)`,
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *fileTransferIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  FileTransferKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *fileTransferIamWrite) getId(data any) *string {
	model, ok := data.(*fileTransferIamWriteModel)
	if !ok {
		return nil
	}

	str := fmt.Sprintf("%s%s", awsPrefix, model.AccountId.ValueString())
	return &str
}

func (r *fileTransferIamWrite) getItemJson(json any) any {
	inner, ok := json.(*fileTransferIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *fileTransferIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := fileTransferIamWriteModel{}
	jsonv, ok := json.(*fileTransferIamWriteJson)
	if !ok {
		return nil
	}

	// remove the aws prefix.
	accountId := strings.TrimPrefix(id, awsPrefix)
	data.AccountId = types.StringValue(accountId)
	data.BucketName = types.StringValue(jsonv.BucketName)
	data.Region = types.StringValue(jsonv.Region)
	data.AwsPartition = types.StringValue(jsonv.AwsPartition)

	data.State = types.StringNull()
	if jsonv.State != nil {
		data.State = types.StringValue(*jsonv.State)
	}

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		label := types.StringValue(*jsonv.Label)
		data.Label = label
	}

	return &data
}

func (r *fileTransferIamWrite) toJson(data any) any {
	json := fileTransferIamWriteJson{}

	datav, ok := data.(*fileTransferIamWriteModel)
	if !ok {
		return nil
	}

	json.BucketName = datav.BucketName.ValueString()
	json.Region = datav.Region.ValueString()
	json.AwsPartition = datav.AwsPartition.ValueString()

	// can omit state and label here as they're filled by the backend
	return &json
}

// Create implements resource.ResourceWithImportState.
func (s *fileTransferIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json fileTransferIamWriteApi
	var data fileTransferIamWriteModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)

	// Convert the model to JSON for the Stage call so the configuration fields are
	// present from the first install step.
	inputJson := s.toJson(&data)

	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *fileTransferIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &fileTransferIamWriteApi{}, &fileTransferIamWriteModel{})
}

func (s *fileTransferIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &fileTransferIamWriteModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *fileTransferIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &fileTransferIamWriteApi{}, &fileTransferIamWriteModel{})
}

func (s *fileTransferIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}
