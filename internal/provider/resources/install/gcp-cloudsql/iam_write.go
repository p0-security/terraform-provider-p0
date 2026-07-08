package installgcpcloudsql

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &GcpCloudSqlIamWrite{}
var _ resource.ResourceWithConfigure = &GcpCloudSqlIamWrite{}
var _ resource.ResourceWithImportState = &GcpCloudSqlIamWrite{}

type GcpCloudSqlIamWrite struct {
	installer *common.Install
}

func NewGcpCloudSqlIamWrite() resource.Resource {
	return &GcpCloudSqlIamWrite{}
}

func (*GcpCloudSqlIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_cloudsql"
}

func (*GcpCloudSqlIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GCP CloudSQL installation.

Installing GCP CloudSQL allows P0 to manage just-in-time access to your CloudSQL (PostgreSQL / MySQL) database instances using GCP IAM authentication.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_gcp_cloudsql_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is reachable.

`,
		Attributes: attributes(),
	}
}

func (r *GcpCloudSqlIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpCloudSqlKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        getId,
		GetItemJson:  getItemJson,
		FromJson:     fromJson,
		ToJson:       toJson,
	}
}

func (r *GcpCloudSqlIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpCloudSqlIamWriteApi
	var data gcpCloudSqlIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpCloudSqlIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpCloudSqlIamWriteApi{}, &gcpCloudSqlIamWriteModel{})
}

func (r *GcpCloudSqlIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpCloudSqlIamWriteApi{}, &gcpCloudSqlIamWriteModel{})
}

func (r *GcpCloudSqlIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gcpCloudSqlIamWriteModel{})
}

func (r *GcpCloudSqlIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
