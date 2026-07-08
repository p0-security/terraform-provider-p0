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

var _ resource.Resource = &GcpCloudSqlIamWriteStaged{}
var _ resource.ResourceWithConfigure = &GcpCloudSqlIamWriteStaged{}
var _ resource.ResourceWithImportState = &GcpCloudSqlIamWriteStaged{}

type GcpCloudSqlIamWriteStaged struct {
	installer *common.Install
}

func NewGcpCloudSqlIamWriteStaged() resource.Resource {
	return &GcpCloudSqlIamWriteStaged{}
}

func (*GcpCloudSqlIamWriteStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_cloudsql_staged"
}

func (*GcpCloudSqlIamWriteStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GCP CloudSQL installation. Staging generates the connector identifiers needed to deploy P0's Cloud Run connector.

Use the read-only ` + "`connector_service_name`" + ` and ` + "`connector_service_account`" + ` attributes to deploy the connector's Cloud Run service (for example via the ` + "`p0-connector/gcp`" + ` module). Once the connector is deployed, create a ` + "`p0_gcp_cloudsql`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Note:** This integration is currently in preview.`,
		Attributes: attributes(),
	}
}

func (r *GcpCloudSqlIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GcpCloudSqlIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpCloudSqlIamWriteApi
	var data gcpCloudSqlIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GcpCloudSqlIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpCloudSqlIamWriteApi{}, &gcpCloudSqlIamWriteModel{})
}

func (r *GcpCloudSqlIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gcpCloudSqlIamWriteApi
	var data gcpCloudSqlIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GcpCloudSqlIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpCloudSqlIamWriteModel{})
}

func (r *GcpCloudSqlIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
