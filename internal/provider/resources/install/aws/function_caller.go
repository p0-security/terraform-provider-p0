package installaws

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AwsFunctionCaller{}
var _ resource.ResourceWithConfigure = &AwsFunctionCaller{}
var _ resource.ResourceWithImportState = &AwsFunctionCaller{}

type AwsFunctionCaller struct {
	installer *common.Install
}

type awsFunctionCallerModel struct {
	Id    types.String `tfsdk:"id"`
	Label types.String `tfsdk:"label"`
	State types.String `tfsdk:"state"`
}

type awsFunctionCallerJson struct {
	Label *string `json:"label,omitempty"`
	State *string `json:"state,omitempty"`
}

type awsFunctionCallerApi struct {
	Item *awsFunctionCallerJson `json:"item"`
}

func NewAwsFunctionCaller() resource.Resource {
	return &AwsFunctionCaller{}
}

func (*AwsFunctionCaller) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_function_caller"
}

func (*AwsFunctionCaller) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS function caller, which lets P0 invoke one Lambda function.

**Important:** Before you create this resource, stage the installation with ` + "`p0_aws_function_caller_staged`" + ` and create its role, inline policy and reader policy in the function's account. Creating this resource runs P0's install check, which fails until the role has both policies, so make it depend on them.

For the full install steps, see [AWS Lambda notifier](https://docs.p0.dev/integrations/notifier-integrations/custom-notifiers/aws-lambda-notifier).`,
		Attributes: map[string]schema.Attribute{
			"id": functionCallerIdAttribute("The `id` of the `p0_aws_function_caller_staged` resource being finalized"),
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: AwsLabelMarkdownDescription,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *AwsFunctionCaller) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        Aws,
		Component:          FunctionCaller,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeFunctionCallerCheckError,
	}
}

func (r *AwsFunctionCaller) getId(data any) *string {
	model, ok := data.(*awsFunctionCallerModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *AwsFunctionCaller) getItemJson(json any) any {
	inner, ok := json.(*awsFunctionCallerApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsFunctionCaller) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*awsFunctionCallerJson)
	if !ok {
		return nil
	}
	return &awsFunctionCallerModel{
		Id:    types.StringValue(id),
		Label: types.StringPointerValue(jsonv.Label),
		State: types.StringPointerValue(jsonv.State),
	}
}

// P0 assembles everything the item holds, and sets its label on verify, so the steps
// send nothing.
func (r *AwsFunctionCaller) toJson(data any) any {
	if _, ok := data.(*awsFunctionCallerModel); !ok {
		return nil
	}
	return &awsFunctionCallerJson{}
}

func (r *AwsFunctionCaller) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsFunctionCallerApi{}, &awsFunctionCallerModel{})
}

func (r *AwsFunctionCaller) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &awsFunctionCallerApi{}, &awsFunctionCallerModel{})
}

// The id is the only attribute that can be set, and it replaces the resource, so an
// update only runs the install steps again.
func (r *AwsFunctionCaller) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsFunctionCallerApi{}, &awsFunctionCallerModel{})
}

func (r *AwsFunctionCaller) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &awsFunctionCallerModel{})
}

func (r *AwsFunctionCaller) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
