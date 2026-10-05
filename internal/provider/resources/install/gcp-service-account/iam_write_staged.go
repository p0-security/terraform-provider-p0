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

var _ resource.Resource = &GcpServiceAccountIamWriteStaged{}
var _ resource.ResourceWithConfigure = &GcpServiceAccountIamWriteStaged{}
var _ resource.ResourceWithImportState = &GcpServiceAccountIamWriteStaged{}

type GcpServiceAccountIamWriteStaged struct {
	installer *common.Install
}

type gcpServiceAccountIamWriteStagedModel struct {
	ProjectId               types.String `tfsdk:"project_id"`
	Region                  types.String `tfsdk:"region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	State                   types.String `tfsdk:"state"`
}

type gcpServiceAccountIamWriteStagedJson struct {
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type gcpServiceAccountIamWriteStagedApi struct {
	Item *gcpServiceAccountIamWriteStagedJson `json:"item"`
}

func NewGcpServiceAccountIamWriteStaged() resource.Resource {
	return &GcpServiceAccountIamWriteStaged{}
}

func (*GcpServiceAccountIamWriteStaged) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_service_account_creator_staged"
}

func (*GcpServiceAccountIamWriteStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged GCP Service Account Creator installation for a Google Cloud project. Staging generates the identifiers needed to deploy P0's service account connector in that project.

Use the read-only ` + "`connector_service_name`" + `, ` + "`connector_service_account`" + ` and ` + "`region`" + ` attributes to deploy the connector's Cloud Run service, its service account, and the custom role it holds. Once the connector is deployed, create a ` + "`p0_gcp_service_account_creator`" + ` resource with the same ` + "`project_id`" + ` to complete the installation.

**Prerequisites:** GCP IAM management (` + "`p0_gcp_iam_write`" + `) and GCP Workload Identity Federation (` + "`p0_gcp_wif_identity`" + `) must be installed for the same project.

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
				MarkdownDescription: `The Google Cloud region to deploy the connector's Cloud Run service in`,
			},
			"connector_service_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name to give the connector's Cloud Run service`,
			},
			"connector_service_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the service account the connector runs as`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GcpServiceAccountIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *GcpServiceAccountIamWriteStaged) getId(data any) *string {
	model, ok := data.(*gcpServiceAccountIamWriteStagedModel)
	if !ok {
		return nil
	}
	str := model.ProjectId.ValueString()
	return &str
}

func (r *GcpServiceAccountIamWriteStaged) getItemJson(json any) any {
	inner, ok := json.(*gcpServiceAccountIamWriteStagedApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GcpServiceAccountIamWriteStaged) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := gcpServiceAccountIamWriteStagedModel{}
	jsonv, ok := json.(*gcpServiceAccountIamWriteStagedJson)
	if !ok {
		return nil
	}

	data.ProjectId = types.StringValue(id)
	data.Region = types.StringPointerValue(jsonv.ConnectorRegion)
	data.ConnectorServiceName = types.StringPointerValue(jsonv.ConnectorServiceName)
	data.ConnectorServiceAccount = types.StringPointerValue(jsonv.ConnectorServiceAccount)
	data.State = types.StringPointerValue(jsonv.State)

	return &data
}

func (r *GcpServiceAccountIamWriteStaged) toJson(data any) any {
	if _, ok := data.(*gcpServiceAccountIamWriteStagedModel); !ok {
		return nil
	}
	// The project is the item id, and P0 assigns every other field, so the body is
	// empty.
	return &gcpServiceAccountIamWriteStagedJson{}
}

func (r *GcpServiceAccountIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpServiceAccountIamWriteStagedApi
	var data gcpServiceAccountIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *GcpServiceAccountIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpServiceAccountIamWriteStagedApi{}, &gcpServiceAccountIamWriteStagedModel{})
}

func (r *GcpServiceAccountIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json gcpServiceAccountIamWriteStagedApi
	var data gcpServiceAccountIamWriteStagedModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
}

func (r *GcpServiceAccountIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpServiceAccountIamWriteStagedModel{})
}

func (r *GcpServiceAccountIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}
