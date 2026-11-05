package installaws

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AwsFunctionCallerStaged{}
var _ resource.ResourceWithConfigure = &AwsFunctionCallerStaged{}
var _ resource.ResourceWithImportState = &AwsFunctionCallerStaged{}

type AwsFunctionCallerStaged struct {
	installer *common.Install
}

type awsFunctionCallerStagedModel struct {
	Id               types.String `tfsdk:"id"`
	AccountId        types.String `tfsdk:"account_id"`
	Partition        types.String `tfsdk:"partition"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Role             types.Object `tfsdk:"role"`
	State            types.String `tfsdk:"state"`
}

// P0 assembles the account and partition from the ARN, so staging sends nothing.
type awsFunctionCallerStagedJson struct {
	AccountId    *string `json:"accountId,omitempty"`
	AwsPartition *string `json:"awsPartition,omitempty"`
	State        *string `json:"state,omitempty"`
}

type awsFunctionCallerStagedApi struct {
	Item     *awsFunctionCallerStagedJson `json:"item"`
	Metadata struct {
		InlinePolicy     string `json:"inlinePolicy"`
		InlinePolicyName string `json:"inlinePolicyName"`
		RoleName         string `json:"roleName"`
		ServiceAccountId string `json:"serviceAccountId"`
		TrustPolicy      string `json:"trustPolicy"`
	} `json:"metadata"`
}

var functionCallerRoleTypes = map[string]attr.Type{
	"name":               types.StringType,
	"inline_policy":      types.StringType,
	"inline_policy_name": types.StringType,
	"trust_policy":       types.StringType,
}

func NewAwsFunctionCallerStaged() resource.Resource {
	return &AwsFunctionCallerStaged{}
}

func (*AwsFunctionCallerStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_function_caller_staged"
}

func (*AwsFunctionCallerStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS function caller, which lets P0 invoke one Lambda function. Staging gives you the IAM role to create in the function's account.

Create the role from the read-only ` + "`role`" + ` attributes, then create a ` + "`p0_aws_function_caller`" + ` resource with the same ` + "`id`" + ` to complete the installation.

Notifiers such as ` + "`p0_aws_lambda_notifier`" + ` call the function through it.

For the full install steps, see [AWS Lambda notifier](https://docs.p0.dev/integrations/notifier-integrations/custom-notifiers/aws-lambda-notifier).`,
		Attributes: map[string]schema.Attribute{
			"id": functionCallerIdAttribute("The ARN of the Lambda function P0 invokes"),
			"account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The function's AWS account ID, from its ARN",
			},
			"partition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The function's AWS partition (`aws` or `aws-us-gov`), from its ARN",
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The audience ID of the service account to include in the role's trust policy",
			},
			"role": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The AWS role that P0 assumes to invoke the function",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The AWS role name",
					},
					"inline_policy": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The inline policy to attach to the role, which allows it to invoke the function",
					},
					"inline_policy_name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The name of the inline policy",
					},
					"trust_policy": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The role's trust policy",
					},
				},
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *AwsFunctionCallerStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  Aws,
		Component:    FunctionCaller,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsFunctionCallerStaged) getId(data any) *string {
	model, ok := data.(*awsFunctionCallerStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// The role comes from the response's metadata, so this keeps the whole response.
func (r *AwsFunctionCallerStaged) getItemJson(json any) any {
	inner, ok := json.(*awsFunctionCallerStagedApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

func (r *AwsFunctionCallerStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*awsFunctionCallerStagedApi)
	if !ok {
		return nil
	}

	role, roleDiags := types.ObjectValue(functionCallerRoleTypes, map[string]attr.Value{
		"name":               types.StringValue(jsonv.Metadata.RoleName),
		"inline_policy":      types.StringValue(jsonv.Metadata.InlinePolicy),
		"inline_policy_name": types.StringValue(jsonv.Metadata.InlinePolicyName),
		"trust_policy":       types.StringValue(jsonv.Metadata.TrustPolicy),
	})
	diags.Append(roleDiags...)
	if roleDiags.HasError() {
		return nil
	}

	return &awsFunctionCallerStagedModel{
		Id:               types.StringValue(id),
		AccountId:        types.StringPointerValue(jsonv.Item.AccountId),
		Partition:        types.StringPointerValue(jsonv.Item.AwsPartition),
		ServiceAccountId: types.StringValue(jsonv.Metadata.ServiceAccountId),
		Role:             role,
		State:            types.StringPointerValue(jsonv.Item.State),
	}
}

func (r *AwsFunctionCallerStaged) toJson(data any) any {
	if _, ok := data.(*awsFunctionCallerStagedModel); !ok {
		return nil
	}
	return &awsFunctionCallerStagedJson{}
}

func (r *AwsFunctionCallerStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.stage(ctx, &req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *AwsFunctionCallerStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &awsFunctionCallerStagedApi{}, &awsFunctionCallerStagedModel{})
}

// Every attribute that can be set replaces the resource, so an update only restages.
func (r *AwsFunctionCallerStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.stage(ctx, &req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *AwsFunctionCallerStaged) stage(ctx context.Context, plan *tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var data awsFunctionCallerStagedModel
	r.installer.EnsureConfig(ctx, diags, plan, state, &data)
	if diags.HasError() {
		return
	}
	r.installer.Stage(ctx, diags, plan, state, &awsFunctionCallerStagedApi{}, &data, r.toJson(&data))
}

func (r *AwsFunctionCallerStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &awsFunctionCallerStagedModel{})
}

func (r *AwsFunctionCallerStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
