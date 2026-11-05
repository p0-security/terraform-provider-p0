package installawslambdanotifier

import (
	"context"
	"regexp"
	"strings"

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
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

const (
	AwsLambdaNotifierKey = "aws-lambda-notifier"
	Notify               = "notify"
)

// P0 names a function caller by its integration, then the item's id: aws:<Lambda ARN>.
const functionCallerPrefix = installaws.Aws + ":"

// The id is a segment of P0's install API path.
var idRegex = regexp.MustCompile(`^[^/]+$`)

var _ resource.Resource = &AwsLambdaNotifier{}
var _ resource.ResourceWithConfigure = &AwsLambdaNotifier{}
var _ resource.ResourceWithImportState = &AwsLambdaNotifier{}

type AwsLambdaNotifier struct {
	installer *common.Install
}

type awsLambdaNotifierModel struct {
	Id             types.String `tfsdk:"id"`
	FunctionCaller types.String `tfsdk:"function_caller"`
	State          types.String `tfsdk:"state"`
}

type awsLambdaNotifierJson struct {
	FunctionCaller *string `json:"functionCaller,omitempty"`
	State          *string `json:"state,omitempty"`
}

type awsLambdaNotifierApi struct {
	Item *awsLambdaNotifierJson `json:"item"`
}

func NewAwsLambdaNotifier() resource.Resource {
	return &AwsLambdaNotifier{}
}

func (*AwsLambdaNotifier) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_lambda_notifier"
}

func (*AwsLambdaNotifier) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS Lambda notifier. P0 invokes the function when a pre-approval is created and when it expires.

**Prerequisite:** P0 must be able to invoke the function, through a ` + "`p0_aws_function_caller`" + ` resource. Set ` + "`function_caller`" + ` to that resource's ` + "`id`" + `, so that Terraform creates this notifier after it.

For the full install steps, see [AWS Lambda notifier](https://docs.p0.dev/integrations/notifier-integrations/custom-notifiers/aws-lambda-notifier).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name P0 gives the notifier. P0 sends it to the function as the event's `id`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(idRegex, "The notifier's name can't contain a /"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"function_caller": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ARN of the Lambda function to invoke, the `id` of an installed `p0_aws_function_caller`",
				Validators:          []validator.String{installaws.LambdaArnValidator},
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *AwsLambdaNotifier) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AwsLambdaNotifierKey,
		Component:    Notify,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsLambdaNotifier) getId(data any) *string {
	model, ok := data.(*awsLambdaNotifierModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *AwsLambdaNotifier) getItemJson(json any) any {
	inner, ok := json.(*awsLambdaNotifierApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsLambdaNotifier) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*awsLambdaNotifierJson)
	if !ok {
		return nil
	}

	functionCaller := types.StringNull()
	if jsonv.FunctionCaller != nil {
		functionCaller = types.StringValue(strings.TrimPrefix(*jsonv.FunctionCaller, functionCallerPrefix))
	}
	return &awsLambdaNotifierModel{
		Id:             types.StringValue(id),
		FunctionCaller: functionCaller,
		State:          types.StringPointerValue(jsonv.State),
	}
}

func (r *AwsLambdaNotifier) toJson(data any) any {
	datav, ok := data.(*awsLambdaNotifierModel)
	if !ok {
		return nil
	}

	json := awsLambdaNotifierJson{}
	if !datav.FunctionCaller.IsNull() && !datav.FunctionCaller.IsUnknown() {
		functionCaller := functionCallerPrefix + datav.FunctionCaller.ValueString()
		json.FunctionCaller = &functionCaller
	}
	return &json
}

// Nothing is deployed between staging and configuring a notifier, so one resource
// takes it through every step.
func (r *AwsLambdaNotifier) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data awsLambdaNotifierModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsLambdaNotifierApi{}, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsLambdaNotifierApi{}, &awsLambdaNotifierModel{})
}

func (r *AwsLambdaNotifier) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &awsLambdaNotifierApi{}, &awsLambdaNotifierModel{})
}

// P0 takes a new function on the configure step, so an installed notifier changes in
// place, without being staged again. One that a failed apply left before "configure"
// goes through every step, as Create does.
func (r *AwsLambdaNotifier) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsLambdaNotifierApi{}, &awsLambdaNotifierModel{})
	default:
		r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsLambdaNotifierApi{}, &awsLambdaNotifierModel{})
	}
}

func (r *AwsLambdaNotifier) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &awsLambdaNotifierModel{})
}

func (r *AwsLambdaNotifier) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
