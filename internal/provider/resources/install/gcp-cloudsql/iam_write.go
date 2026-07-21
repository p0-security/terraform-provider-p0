package installgcpcloudsql

import (
	"context"

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
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

var _ resource.Resource = &GcpCloudSqlIamWrite{}
var _ resource.ResourceWithConfigure = &GcpCloudSqlIamWrite{}
var _ resource.ResourceWithImportState = &GcpCloudSqlIamWrite{}

type GcpCloudSqlIamWrite struct {
	installer *common.Install
}

type gcpCloudSqlIamWriteModel struct {
	Id                      types.String `tfsdk:"id"`
	ProjectId               types.String `tfsdk:"project_id"`
	Subnetwork              types.String `tfsdk:"subnetwork"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceUri     types.String `tfsdk:"connector_service_uri"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	State                   types.String `tfsdk:"state"`
}

type gcpCloudSqlIamWriteJson struct {
	ProjectId               string  `json:"projectId"`
	ConnectorSubnetwork     *string `json:"connectorSubnetwork,omitempty"`
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	State                   string  `json:"state"`
}

type gcpCloudSqlIamWriteApi struct {
	Item *gcpCloudSqlIamWriteJson `json:"item"`
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

Installing GCP CloudSQL allows P0 to manage just-in-time access to your CloudSQL (PostgreSQL) database instances (MySQL is not yet supported) using GCP IAM authentication.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_gcp_cloudsql_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that the connector is reachable.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The GCP VPC (network) identifier for this CloudSQL installation`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The GCP project in which this VPC is provisioned`,
				Validators: []validator.String{
					// Reuse the shared GCP project-id validator so that a project
					// (selected from an already-installed gcloud iam-write install)
					// validates consistently across all p0_gcp_* integrations.
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subnetwork": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The name of the subnetwork to which the Cloud Run connector should have direct VPC access`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The GCP region in which the connector's Cloud Run service is provisioned (defaults to us-west1)`,
			},
			"connector_service_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name of the connector's GCP Cloud Run service`,
			},
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The invocation URL of the connector's Cloud Run service (resolved once the connector is installed)`,
			},
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The GCP service account that the connector runs as`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GcpCloudSqlIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpCloudSqlKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GcpCloudSqlIamWrite) getId(data any) *string {
	model, ok := data.(*gcpCloudSqlIamWriteModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GcpCloudSqlIamWrite) getItemJson(json any) any {
	inner, ok := json.(*gcpCloudSqlIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GcpCloudSqlIamWrite) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := gcpCloudSqlIamWriteModel{}
	jsonv, ok := json.(*gcpCloudSqlIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.ProjectId = types.StringValue(jsonv.ProjectId)
	data.State = types.StringValue(jsonv.State)
	data.Subnetwork = types.StringPointerValue(jsonv.ConnectorSubnetwork)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceUri = types.StringPointerValue(jsonv.ConnectorServiceUri)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)

	return &data
}

func (r *GcpCloudSqlIamWrite) toJson(data any) any {
	json := gcpCloudSqlIamWriteJson{}
	datav, ok := data.(*gcpCloudSqlIamWriteModel)
	if !ok {
		return nil
	}
	// projectId and the subnetwork are user-owned inputs; region and
	// the connector_* fields are assigned by the backend, so they are
	// intentionally omitted from the request.
	json.ProjectId = datav.ProjectId.ValueString()
	subnetwork := datav.Subnetwork.ValueString()
	json.ConnectorSubnetwork = &subnetwork
	return &json
}

func (r *GcpCloudSqlIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpCloudSqlIamWriteApi
	var data gcpCloudSqlIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := r.toJson(&data)
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
