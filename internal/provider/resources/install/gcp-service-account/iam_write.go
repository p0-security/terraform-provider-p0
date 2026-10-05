package installgcpserviceaccount

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

var _ resource.Resource = &GcpServiceAccountIamWrite{}
var _ resource.ResourceWithConfigure = &GcpServiceAccountIamWrite{}
var _ resource.ResourceWithImportState = &GcpServiceAccountIamWrite{}

type GcpServiceAccountIamWrite struct {
	installer *common.Install
}

type gcpServiceAccountIamWriteModel struct {
	ProjectId               types.String `tfsdk:"project_id"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	ConnectorServiceUri     types.String `tfsdk:"connector_service_uri"`
	State                   types.String `tfsdk:"state"`
}

type gcpServiceAccountIamWriteJson struct {
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type gcpServiceAccountIamWriteApi struct {
	Item *gcpServiceAccountIamWriteJson `json:"item"`
}

func NewGcpServiceAccountIamWrite() resource.Resource {
	return &GcpServiceAccountIamWrite{}
}

func (*GcpServiceAccountIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_service_account_creator"
}

func (*GcpServiceAccountIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GCP Service Account Creator installation for a Google Cloud project.

Installing the GCP Service Account Creator lets P0 create a short-lived service account in this project for each agentic session, so that integrations such as Google Drive (` + "`p0_google_drive_connector`" + `) can grant access to it.

**Important:** Before creating this resource you must stage the installation with ` + "`p0_gcp_service_account_creator_staged`" + ` and deploy the connector's Cloud Run service. Creating this resource verifies that P0 can reach the connector.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Google Cloud project P0 creates agentic identities in. The connector runs in this project too.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The Google Cloud region the connector's Cloud Run service runs in`,
			},
			"connector_service_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name of the connector's Cloud Run service`,
			},
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the service account the connector runs as`,
			},
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The invocation URL of the connector's Cloud Run service`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GcpServiceAccountIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpServiceAccountKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

// The item is keyed by the project.
func (r *GcpServiceAccountIamWrite) getId(data any) *string {
	model, ok := data.(*gcpServiceAccountIamWriteModel)
	if !ok {
		return nil
	}
	str := model.ProjectId.ValueString()
	return &str
}

func (r *GcpServiceAccountIamWrite) getItemJson(json any) any {
	inner, ok := json.(*gcpServiceAccountIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GcpServiceAccountIamWrite) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := gcpServiceAccountIamWriteModel{}
	jsonv, ok := json.(*gcpServiceAccountIamWriteJson)
	if !ok {
		return nil
	}

	data.ProjectId = types.StringValue(id)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)
	data.ConnectorServiceUri = types.StringPointerValue(jsonv.ConnectorServiceUri)
	data.State = types.StringPointerValue(jsonv.State)

	return &data
}

func (r *GcpServiceAccountIamWrite) toJson(data any) any {
	if _, ok := data.(*gcpServiceAccountIamWriteModel); !ok {
		return nil
	}
	// The project is the item id, and P0 assigns every other field, so the body is
	// empty.
	return &gcpServiceAccountIamWriteJson{}
}

func (r *GcpServiceAccountIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpServiceAccountIamWriteApi
	var data gcpServiceAccountIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpServiceAccountIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpServiceAccountIamWriteApi{}, &gcpServiceAccountIamWriteModel{})
}

func (r *GcpServiceAccountIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpServiceAccountIamWriteApi{}, &gcpServiceAccountIamWriteModel{})
}

// Returns the item to the "stage" state rather than deleting it, so that the staged
// resource still owns it.
func (r *GcpServiceAccountIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &gcpServiceAccountIamWriteModel{})
}

func (r *GcpServiceAccountIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
