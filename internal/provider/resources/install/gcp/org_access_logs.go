package installgcp

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

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpOrgAccessLogs{}
var _ resource.ResourceWithImportState = &GcpOrgAccessLogs{}
var _ resource.ResourceWithConfigure = &GcpOrgAccessLogs{}

func NewGcpOrgAccessLogs() resource.Resource {
	return &GcpOrgAccessLogs{}
}

type GcpOrgAccessLogs struct {
	installer *common.Install
}

type gcpOrgAccessLogsModel struct {
	State          types.String `tfsdk:"state"`
	TopicProjectId types.String `tfsdk:"topic_project_id"`
}

type gcpOrgAccessLogsJson struct {
	State          string `json:"state"`
	TopicProjectId string `json:"topicProjectId"`
}

type gcpOrgAccessLogsApi struct {
	Item gcpOrgAccessLogsJson `json:"item"`
}

func (r *GcpOrgAccessLogs) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_organization_access_logs"
}

func (r *GcpOrgAccessLogs) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on an entire Google Cloud organization, for access-log collection,
which enhances IAM assessment. Note that P0 will have access to logs from all your projects, not just those
configured for IAM assessment.

To use this resource, you must also:
- grant P0 the ability to create logging sinks on your organization.

Use the read-only attributes defined on ` + "`p0_gcp`" + ` to create the requisite Google Cloud infrastructure.

P0 recommends defining this infrastructure according to the example usage pattern.`,
		Attributes: map[string]schema.Attribute{
			"state": common.StateAttribute,
			"topic_project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The project identifier where the access-logs Pub/Sub topic should reside`,
				Validators:          projectValidators,
			},
		},
	}
}

func (r *GcpOrgAccessLogs) getItemJson(json any) any {
	inner, ok := json.(*gcpOrgAccessLogsApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *GcpOrgAccessLogs) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpOrgAccessLogsModel{}
	jsonv, ok := json.(*gcpOrgAccessLogsJson)
	if !ok {
		return nil
	}

	data.State = types.StringValue(jsonv.State)
	data.TopicProjectId = types.StringValue(jsonv.TopicProjectId)

	return &data
}

func (r *GcpOrgAccessLogs) toJson(data any) any {
	json := gcpOrgAccessLogsJson{}
	datav, ok := data.(*gcpOrgAccessLogsModel)
	if !ok {
		return nil
	}

	// can omit state here as it's filled by the backend
	json.TopicProjectId = datav.TopicProjectId.ValueString()
	return json
}

func (r *GcpOrgAccessLogs) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpKey,
		Component:    OrgAccessLogs,
		ProviderData: providerData,
		GetId:        singletonGetId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *GcpOrgAccessLogs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpOrgAccessLogsApi
	var data gcpOrgAccessLogsModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *GcpOrgAccessLogs) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpOrgAccessLogsApi{}, &gcpOrgAccessLogsModel{})
}

func (s *GcpOrgAccessLogs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpOrgAccessLogsModel{})
}

func (s *GcpOrgAccessLogs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpOrgAccessLogsApi{}, &gcpOrgAccessLogsModel{})
}

func (s *GcpOrgAccessLogs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Empty(), req, resp)
}
