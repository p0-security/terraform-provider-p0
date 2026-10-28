package installdynamodbai

import (
	"context"
	"regexp"

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

const DynamoDbAiKey = "dynamodb-ai"

var awsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &dynamoDbAiIamWrite{}
var _ resource.ResourceWithConfigure = &dynamoDbAiIamWrite{}
var _ resource.ResourceWithImportState = &dynamoDbAiIamWrite{}

// copy of rdsIamWrite in internal/provider/resources/install/rds/iam_write.go,
// without account_id and region: the id is the AWS account ID.
type dynamoDbAiIamWrite struct {
	installer *common.Install
}

type dynamoDbAiIamWriteModel struct {
	Id    types.String `tfsdk:"id" json:"id,omitempty"`
	State types.String `tfsdk:"state" json:"state,omitempty"`
	Label types.String `tfsdk:"label" json:"label,omitempty"`
}

type dynamoDbAiIamWriteJson struct {
	State string  `json:"state"`
	Label *string `json:"label,omitempty"`
}

type dynamoDbAiIamWriteApi struct {
	Item *dynamoDbAiIamWriteJson `json:"item"`
}

func NewDynamoDbAiIamWrite() resource.Resource {
	return &dynamoDbAiIamWrite{}
}

func (*dynamoDbAiIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_dynamodb_ai"
}

func (*dynamoDbAiIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A DynamoDB (AI) installation for one AWS account.

Installing this resource lets agents in P0 agentic sessions request time-bound read or write access to the account's DynamoDB tables. It adds nothing in AWS.

**Important:** P0's AWS IAM management (see the p0_aws_iam_write resource) and AWS OIDC federation (see the p0_aws_oidc_identity resource) must be installed for the account first.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(awsAccountIdRegex, "AWS account IDs should be 12 digits"),
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
				MarkdownDescription: "The AWS account's alias (if available)",
				Computed:            true,
				Optional:            true,
			},
		},
	}
}

func (r *dynamoDbAiIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  DynamoDbAiKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *dynamoDbAiIamWrite) getId(data any) *string {
	model, ok := data.(*dynamoDbAiIamWriteModel)
	if !ok {
		return nil
	}

	str := model.Id.ValueString()
	return &str
}

func (r *dynamoDbAiIamWrite) getItemJson(json any) any {
	inner, ok := json.(*dynamoDbAiIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *dynamoDbAiIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := dynamoDbAiIamWriteModel{}
	jsonv, ok := json.(*dynamoDbAiIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.State = types.StringValue(jsonv.State)

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	return &data
}

func (r *dynamoDbAiIamWrite) toJson(data any) any {
	json := dynamoDbAiIamWriteJson{}

	datav, ok := data.(*dynamoDbAiIamWriteModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	// can omit state here as it's filled by the backend
	return &json
}

func (s *dynamoDbAiIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json dynamoDbAiIamWriteApi
	var data dynamoDbAiIamWriteModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, s.toJson(&data))
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *dynamoDbAiIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &dynamoDbAiIamWriteApi{}, &dynamoDbAiIamWriteModel{})
}

func (s *dynamoDbAiIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &dynamoDbAiIamWriteModel{})
}

func (s *dynamoDbAiIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &dynamoDbAiIamWriteApi{}, &dynamoDbAiIamWriteModel{})
}

func (s *dynamoDbAiIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
