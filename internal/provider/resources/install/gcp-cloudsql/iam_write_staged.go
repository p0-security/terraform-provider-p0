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

var _ resource.Resource = &GcpCloudSqlIamWriteStaged{}
var _ resource.ResourceWithConfigure = &GcpCloudSqlIamWriteStaged{}
var _ resource.ResourceWithImportState = &GcpCloudSqlIamWriteStaged{}

type GcpCloudSqlIamWriteStaged struct {
	installer *common.Install
}

type gcpCloudSqlIamWriteStagedModel struct {
	Id                      types.String `tfsdk:"id"`
	ProjectId               types.String `tfsdk:"project_id"`
	Subnetwork              types.String `tfsdk:"subnetwork"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	State                   types.String `tfsdk:"state"`
}

type gcpCloudSqlIamWriteStagedJson struct {
	ProjectId               string  `json:"projectId"`
	ConnectorSubnetwork     *string `json:"connectorSubnetwork,omitempty"`
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	State                   *string `json:"state"`
}

type gcpCloudSqlIamWriteStagedApi struct {
	Item *gcpCloudSqlIamWriteStagedJson `json:"item"`
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

Use the read-only ` + "`connector_service_name`" + ` and ` + "`connector_service_account`" + ` attributes to deploy the connector's Cloud Run service (for example via the ` + "`p0-security/p0-connector/google`" + ` Terraform Registry module, version 0.0.3). Once the connector is deployed, create a ` + "`p0_gcp_cloudsql`" + ` resource with the same ` + "`id`" + ` to complete the installation.

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
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The GCP service account that the connector runs as`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GcpCloudSqlIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GcpCloudSqlIamWriteStaged) getId(data any) *string {
	model, ok := data.(*gcpCloudSqlIamWriteStagedModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GcpCloudSqlIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*gcpCloudSqlIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GcpCloudSqlIamWriteStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := gcpCloudSqlIamWriteStagedModel{}
	jsonv, ok := json.(*gcpCloudSqlIamWriteStagedJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.ProjectId = types.StringValue(jsonv.ProjectId)
	if jsonv.State != nil {
		data.State = types.StringValue(*jsonv.State)
	}
	data.Subnetwork = types.StringPointerValue(jsonv.ConnectorSubnetwork)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)

	return &data
}

func (r *GcpCloudSqlIamWriteStaged) toJson(data any) any {
	json := gcpCloudSqlIamWriteStagedJson{}
	datav, ok := data.(*gcpCloudSqlIamWriteStagedModel)
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

func (r *GcpCloudSqlIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpCloudSqlIamWriteStagedApi
	var data gcpCloudSqlIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GcpCloudSqlIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpCloudSqlIamWriteStagedApi{}, &gcpCloudSqlIamWriteStagedModel{})
}

func (r *GcpCloudSqlIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gcpCloudSqlIamWriteStagedApi
	var data gcpCloudSqlIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
}

func (r *GcpCloudSqlIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpCloudSqlIamWriteStagedModel{})
}

func (r *GcpCloudSqlIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
