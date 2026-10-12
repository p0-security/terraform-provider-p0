package installvaultedcredential

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

const GcpSecretManager = "gcp-sm"

// P0 names the project "install" and sets all connector fields.
type SecretManagerJson struct {
	Type                    string  `json:"type"`
	Install                 string  `json:"install"`
	ConnectorRegion         *string `json:"connectorRegion,omitempty"`
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
}

type SecretManagerStagedModel struct {
	Type                    types.String `tfsdk:"type"`
	ProjectId               types.String `tfsdk:"project_id"`
	ConnectorRegion         types.String `tfsdk:"connector_region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
}

type SecretManagerModel struct {
	SecretManagerStagedModel
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

func (m *SecretManagerStagedModel) ToJson() *SecretManagerJson {
	return &SecretManagerJson{
		Type:    m.Type.ValueString(),
		Install: m.ProjectId.ValueString(),
	}
}

func SecretManagerStagedFromJson(json *SecretManagerJson) SecretManagerStagedModel {
	return SecretManagerStagedModel{
		Type:                    types.StringValue(json.Type),
		ProjectId:               types.StringValue(json.Install),
		ConnectorRegion:         types.StringPointerValue(json.ConnectorRegion),
		ConnectorServiceName:    types.StringPointerValue(json.ConnectorServiceName),
		ConnectorServiceAccount: types.StringPointerValue(json.ConnectorServiceAccount),
	}
}

func SecretManagerFromJson(json *SecretManagerJson) SecretManagerModel {
	return SecretManagerModel{
		SecretManagerStagedModel: SecretManagerStagedFromJson(json),
		ConnectorServiceUri:      types.StringPointerValue(json.ConnectorServiceUri),
	}
}

// secret_manager is required, so a nil value makes Terraform fail with
// "inconsistent result after apply".
//
// The integration label names the install in the error, e.g. "GitHub App".
func RequireSecretManager(diags *diag.Diagnostics, integration string, id string, json *SecretManagerJson) bool {
	if json == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The %s install %s carries no secret manager configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", integration, id),
		)
		return false
	}
	return true
}

func IdAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
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
// The secret label names what the install secret holds, e.g. "the GitHub App's
// private key". Adds suffix to the type and project_id descriptions.
func SecretManagerAttributes(secret string, suffix string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The secret manager that stores ` + secret + `. Only ` + "`gcp-sm`" + ` (Google Secret Manager) is supported.` + suffix,
			Validators: []validator.String{
				stringvalidator.OneOf(GcpSecretManager),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"project_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The GCP project that hosts the connector's Cloud Run service and the secret that holds ` + secret + `.` + suffix,
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

// Adds the connector service URI that P0 resolves when it completes the install.
func FinalSecretManagerAttributes(secret string, suffix string) map[string]schema.Attribute {
	attributes := SecretManagerAttributes(secret, suffix)
	attributes["connector_service_uri"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: `The invocation URL of the connector's Cloud Run service, resolved by P0 during install`,
	}
	return attributes
}
