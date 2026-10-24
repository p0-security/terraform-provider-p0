package installawssqs

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &AuditLogs{}
var _ resource.ResourceWithConfigure = &AuditLogs{}
var _ resource.ResourceWithImportState = &AuditLogs{}

func NewAuditLogs() resource.Resource {
	return &AuditLogs{}
}

type AuditLogs struct {
	installer *common.Install
}

type auditLogsModel struct {
	Id        types.String `tfsdk:"id"`
	KmsKeyArn types.String `tfsdk:"kms_key_arn"`
	State     types.String `tfsdk:"state"`
}

func (r *AuditLogs) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_sqs_audit_logs"
}

func (r *AuditLogs) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Sends P0 audit events to an Amazon SQS queue, one JSON message per event.

To install:
1. Create a ` + "`p0_aws_sqs_audit_logs_staged`" + ` resource for the queue.
2. Create the ` + "`" + RoleName + "`" + ` role in the queue's AWS account. It must trust P0's service account (` + "`service_account_id`" + ` from the staged resource) and allow ` + "`sqs:SendMessage`" + ` on the queue. See the example.
3. Create this resource with the same ` + "`id`" + `. P0 checks the role's policies and sends one test message to the queue.

Only one ` + "`p0_aws_sqs_audit_logs`" + ` resource is supported per P0 organization.

For the full install steps, see [Amazon SQS setup](` + docsUrl + `).`,
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute,
			"kms_key_arn": kmsKeyArnAttribute,
			"state":       common.StateAttribute,
		},
	}
}

func (r *AuditLogs) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        AwsSqsIntegration,
		Component:          AuditLogsComponent,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError,
	}
}

// A failed check almost always means the role, its policies, or the queue don't
// match what P0 expects.
func describeCheckError(id string, err error) (string, string) {
	return "Could not install the Amazon SQS queue",
		fmt.Sprintf("P0 could not verify %s: %v\n\nCheck that the %s role exists in the queue's account, trusts the staged resource's service_account_id, and has the inline policy from the example, and that the queue exists.", id, err, RoleName)
}

func (r *AuditLogs) getId(data any) *string {
	model, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *AuditLogs) getItemJson(json any) any {
	inner, ok := json.(*auditLogsApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AuditLogs) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*auditLogsJson)
	if !ok {
		return nil
	}
	return &auditLogsModel{
		Id:        types.StringValue(id),
		KmsKeyArn: types.StringPointerValue(jsonv.KmsKeyArn),
		State:     types.StringPointerValue(jsonv.State),
	}
}

func (r *AuditLogs) toJson(data any) any {
	datav, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}
	return &auditLogsInputJson{KmsKeyArn: datav.KmsKeyArn.ValueStringPointer()}
}

func (r *AuditLogs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data auditLogsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &auditLogsApi{}, &data)
}

func (r *AuditLogs) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &auditLogsApi{}, &auditLogsModel{})
}

func (r *AuditLogs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data auditLogsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &auditLogsApi{}, &data)
}

// Returns the queue to "stage", so destroying it with the staged resource doesn't
// delete it twice.
func (r *AuditLogs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &resp.State, &auditLogsModel{})
}

func (r *AuditLogs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
