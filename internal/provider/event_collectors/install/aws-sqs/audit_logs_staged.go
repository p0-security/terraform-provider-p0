package installawssqs

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AuditLogsStaged{}
var _ resource.ResourceWithConfigure = &AuditLogsStaged{}
var _ resource.ResourceWithImportState = &AuditLogsStaged{}

func NewAuditLogsStaged() resource.Resource {
	return &AuditLogsStaged{}
}

type AuditLogsStaged struct {
	installer *common.Install
}

type auditLogsStagedModel struct {
	Id                  types.String `tfsdk:"id"`
	KmsKeyArn           types.String `tfsdk:"kms_key_arn"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	State               types.String `tfsdk:"state"`
}

func (r *AuditLogsStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_sqs_audit_logs_staged"
}

func (r *AuditLogsStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Amazon SQS queue for P0 audit logs. Staging returns the P0 service account that the queue's ` + "`" + RoleName + "`" + ` role must trust.

Use ` + "`service_account_id`" + ` to create the ` + "`" + RoleName + "`" + ` role in the queue's AWS account, then create a ` + "`p0_aws_sqs_audit_logs`" + ` resource with the same ` + "`id`" + ` to finish the install.

**Prerequisite:** P0's AWS integration must be installed for the queue's AWS account.

For the full install steps, see [Amazon SQS setup](` + docsUrl + `).`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute,
			"kms_key_arn": kmsKeyArnAttribute,
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique ID of P0's Google service account. Use it as the `accounts.google.com:aud` condition in the role's trust policy.",
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of P0's Google service account",
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *AuditLogsStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AwsSqsIntegration,
		Component:    AuditLogsComponent,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AuditLogsStaged) getId(data any) *string {
	model, ok := data.(*auditLogsStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *AuditLogsStaged) getItemJson(json any) any {
	inner, ok := json.(*auditLogsApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AuditLogsStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*auditLogsJson)
	if !ok {
		return nil
	}
	return &auditLogsStagedModel{
		Id:                  types.StringValue(id),
		KmsKeyArn:           types.StringPointerValue(jsonv.KmsKeyArn),
		ServiceAccountId:    types.StringPointerValue(jsonv.ServiceAccountId),
		ServiceAccountEmail: types.StringPointerValue(jsonv.ServiceAccountEmail),
		State:               types.StringPointerValue(jsonv.State),
	}
}

func (r *AuditLogsStaged) toJson(data any) any {
	datav, ok := data.(*auditLogsStagedModel)
	if !ok {
		return nil
	}
	return &auditLogsInputJson{KmsKeyArn: datav.KmsKeyArn.ValueStringPointer()}
}

func (r *AuditLogsStaged) stage(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State) {
	var json auditLogsApi
	var data auditLogsStagedModel

	r.installer.EnsureConfig(ctx, diags, plan, state, &data)
	if diags.HasError() {
		return
	}
	r.installer.Stage(ctx, diags, plan, state, &json, &data, r.toJson(&data))
}

func (r *AuditLogsStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State)
}

func (r *AuditLogsStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &auditLogsApi{}, &auditLogsStagedModel{})
}

func (r *AuditLogsStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State)
}

func (r *AuditLogsStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &auditLogsStagedModel{})
}

func (r *AuditLogsStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
