package installgcpcloudsql

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

const GcpCloudSqlKey = "gcp-cloudsql"

// All installable GCP CloudSQL components.
var Components = []string{installresources.IamWrite}

// gcpCloudSqlIamWriteModel is the Terraform state model shared by the staged and
// full resources (their schemas are identical).
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

// attributes returns the schema shared by both the staged and full resources.
func attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
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
			Optional:            true,
			Computed:            true,
			MarkdownDescription: `The name of the subnetwork the connector should have direct VPC access to (defaults to the name of the VPC)`,
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
	}
}

func getId(data any) *string {
	model, ok := data.(*gcpCloudSqlIamWriteModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func getItemJson(json any) any {
	inner, ok := json.(*gcpCloudSqlIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
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

func toJson(data any) any {
	json := gcpCloudSqlIamWriteJson{}
	datav, ok := data.(*gcpCloudSqlIamWriteModel)
	if !ok {
		return nil
	}
	// projectId and (optionally) the subnetwork are user-owned inputs; region and
	// the connector_* fields are assigned by the backend, so they are
	// intentionally omitted from the request.
	json.ProjectId = datav.ProjectId.ValueString()
	if !datav.Subnetwork.IsNull() && !datav.Subnetwork.IsUnknown() {
		subnetwork := datav.Subnetwork.ValueString()
		json.ConnectorSubnetwork = &subnetwork
	}
	return &json
}
