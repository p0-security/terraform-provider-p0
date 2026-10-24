package installjiraai

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// The `hosting` attribute is the app's `JiraAiConnectorHosting` install element
// (packages/integrations/jira-ai/shared/src/connector-hosting.ts): the app's
// `ConnectorHosting` with its Cloud Run option alone, plus the service account P0
// names after the connector. P0 stores it in the same JSON as every connector's
// hosting, installapp.ConnectorHostingJson. The beta runs on Cloud Run only, so the
// attribute has no `account_id`, and `type` takes `gcp` alone.
//
// The model, the attribute and the JSON conversions are a Google Cloud-only copy of
// installapp's ConnectorHostingWithServiceAccount (app/hosting.go). The connector
// name rule and message are Harbor's (harbor/connector_item.go).

type hostingModel struct {
	Type                    types.String `tfsdk:"type"`
	ProjectId               types.String `tfsdk:"project_id"`
	ConnectorName           types.String `tfsdk:"connector_name"`
	ConnectorRegion         types.String `tfsdk:"connector_region"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	ConnectorServiceUri     types.String `tfsdk:"connector_service_uri"`
}

// The hosting as P0 stores it. connector_service_account and connector_service_uri
// are left out, because P0 sets both.
func (m *hostingModel) toJson() *installapp.ConnectorHostingJson {
	return &installapp.ConnectorHostingJson{
		Type:            m.Type.ValueString(),
		ProjectId:       m.ProjectId.ValueStringPointer(),
		ConnectorName:   m.ConnectorName.ValueString(),
		ConnectorRegion: m.ConnectorRegion.ValueString(),
	}
}

func hostingFromJson(json *installapp.ConnectorHostingJson) *hostingModel {
	return &hostingModel{
		Type:                    types.StringValue(json.Type),
		ProjectId:               types.StringPointerValue(json.ProjectId),
		ConnectorName:           types.StringValue(json.ConnectorName),
		ConnectorRegion:         types.StringValue(json.ConnectorRegion),
		ConnectorServiceAccount: types.StringPointerValue(json.ConnectorServiceAccount),
		ConnectorServiceUri:     types.StringPointerValue(json.ConnectorServiceUri),
	}
}

// The connector name and region use the app's rules and messages
// (packages/integrations/jira-ai/shared/src/connector-hosting.ts and gcp-hosting.ts).
// The name becomes both the Cloud Run service and its service account ID, so it has to
// be valid for both.
//
// The project rule is stricter than the app's backend, which takes any letters, digits
// and hyphens. It's the rule the app's Terraform generator (frontend/src/iac.tsx)
// applies, since a connector in any other project can't be deployed with that
// Terraform. The message is the provider's own.
//
// None of the patterns allows whitespace. P0 trims what it stores, so a padded value
// would read back different from the configuration.
var (
	gcpProjectIdRegex     = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	gcpConnectorNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	gcpRegionRegex        = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+\d+$`)
)

const (
	invalidGcpProject = "Enter the ID of the Google Cloud project the connector runs in: 6 to 30 lowercase letters, digits and hyphens, " +
		"starting with a letter and ending with a letter or digit, not a project's number or name"
	invalidGcpConnectorName = "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, " +
		"starting with a letter and ending with a letter or digit"
	invalidGcpRegion = "Enter the Google Cloud region the connector runs in, such as us-central1"
)

// The required `hosting` attribute. Every address field is `step: "new"` in the app's
// install schema, and P0 refuses a change to one once the connector is added, so each
// requires replacement: Terraform plans the destroy-and-recreate that works, rather
// than an in-place update that P0 would refuse.
func hostingAttribute(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"type": fixedAttribute(
				"The connector's hosting. Only `gcp` (Google Cloud Run) is supported.",
				stringvalidator.OneOf(installapp.GcpHosting),
			),
			"project_id": fixedAttribute(
				"The ID of the Google Cloud project the connector's Cloud Run service is deployed in.",
				stringvalidator.RegexMatches(gcpProjectIdRegex, invalidGcpProject),
			),
			"connector_name": fixedAttribute(
				"The name of the connector's Cloud Run service. P0 names the service account the connector runs as after it. "+invalidGcpConnectorName+".",
				stringvalidator.RegexMatches(gcpConnectorNameRegex, invalidGcpConnectorName),
			),
			"connector_region": fixedAttribute(
				"The Google Cloud region the connector is deployed in, such as `us-central1`.",
				stringvalidator.RegexMatches(gcpRegionRegex, invalidGcpRegion),
			),
			"connector_service_account": generatedAttribute(
				"The email of the service account the connector's Cloud Run service must run as, which P0 names after the connector when it stages it. " +
					"Each site that uses the connector grants this account access to its secrets.",
			),
			"connector_service_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The connector's invocation URL, which P0 looks up when it verifies the install.",
			},
		},
	}
}
