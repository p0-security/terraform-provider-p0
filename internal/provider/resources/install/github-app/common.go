package installgithubapp

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

const GithubAppKey = "github-app"

// All installable GitHub App components.
var Components = []string{installresources.Credential}

const GcpSecretManager = "gcp-sm"

// P0 names the project "install" and sets all connector fields.
type secretManagerJson struct {
	Type                    string  `json:"type"`
	Install                 string  `json:"install"`
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
}

type secretManagerStagedModel struct {
	Type                    types.String `tfsdk:"type"`
	ProjectId               types.String `tfsdk:"project_id"`
	ConnectorRegion         types.String `tfsdk:"connector_region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
}

type secretManagerModel struct {
	secretManagerStagedModel
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

func (m *secretManagerStagedModel) toJson() *secretManagerJson {
	return &secretManagerJson{
		Type:    m.Type.ValueString(),
		Install: m.ProjectId.ValueString(),
	}
}

func secretManagerStagedFromJson(json *secretManagerJson) secretManagerStagedModel {
	return secretManagerStagedModel{
		Type:                    types.StringValue(json.Type),
		ProjectId:               types.StringValue(json.Install),
		ConnectorRegion:         types.StringPointerValue(json.ConnectorRegion),
		ConnectorServiceName:    types.StringPointerValue(json.ConnectorServiceName),
		ConnectorServiceAccount: types.StringPointerValue(json.ConnectorServiceAccount),
	}
}

// secret_manager is required, so a nil value makes Terraform fail with
// "inconsistent result after apply".
func requireSecretManager(diags *diag.Diagnostics, id string, json *secretManagerJson) bool {
	if json == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The GitHub App install %s carries no secret manager configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return false
	}
	return true
}

func idAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func computedConnectorAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// P0 rejects a change to type or project_id after staging, so a change replaces
// the install.
//
// Adds suffix to the type and project_id descriptions.
func secretManagerAttributes(suffix string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The secret manager that stores the GitHub App's private key. Only ` + "`gcp-sm`" + ` (Google Secret Manager) is supported.` + suffix,
			Validators: []validator.String{
				stringvalidator.OneOf(GcpSecretManager),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"project_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The GCP project that hosts the connector's Cloud Run service and the private key secret.` + suffix,
			Validators: []validator.String{
				stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"connector_region":          computedConnectorAttribute(`The GCP region in which to deploy the connector's Cloud Run service`),
		"connector_service_name":    computedConnectorAttribute(`The name to give the connector's Cloud Run service`),
		"connector_service_account": computedConnectorAttribute(`The email of the GCP service account that the connector must run as`),
	}
}
