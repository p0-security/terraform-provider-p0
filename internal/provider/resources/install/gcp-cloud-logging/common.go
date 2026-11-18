package installgcpcloudlogging

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	logsource "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/log-source"
)

// The Google Cloud Logging log source, as the app defines it in
// packages/integrations/gcp-cloud-logging/shared/src/components.ts.
const (
	GcpCloudLoggingKey    = "gcp-cloud-logging"
	projectComponent      = "project"
	organizationComponent = "organization"
	label                 = "Google Cloud Logging"
)

// The app's rule for a Google Cloud project ID: 6 to 30 lowercase letters,
// digits and hyphens, starting with a letter and not ending with a hyphen.
var projectIdRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

func projectIdValidator() validator.String {
	return stringvalidator.RegexMatches(projectIdRegex, "Enter a Google Cloud project ID, such as my-project-123")
}

// The prerequisites of every Google Cloud Logging resource.
const prerequisiteNote = `**Prerequisite:** P0's Google Cloud integration must be installed for your organization (see ` + "`p0_gcp`" + `). P0 invokes the connector as that installation's service account.`

// The attributes that both components' items have, before P0 installs them.
func connectorAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host_project_id": logsource.FixedAttribute(
			"The Google Cloud project that the connector runs in, on Cloud Run",
			projectIdValidator(),
		),
		"connector_service_name":    logsource.GeneratedAttribute("The name of the connector's Cloud Run service, which P0 generates when it stages the log source"),
		"connector_service_account": logsource.GeneratedAttribute("The email of the service account that the connector runs as, in the host project, which P0 generates when it stages the log source"),
		"region":                    logsource.GeneratedAttribute("The region that the connector's Cloud Run service runs in, which P0 chooses when it stages the log source"),
		"state":                     common.StateAttribute,
	}
}

// The attribute that P0 sets once it finds the deployed connector.
const connectorServiceUriDescription = "The connector's Cloud Run URL, which P0 reads from the deployed service when it installs the log source"

type connectorModel struct {
	HostProjectId           types.String `tfsdk:"host_project_id"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
	Region                  types.String `tfsdk:"region"`
	State                   types.String `tfsdk:"state"`
}

// Both components' items record the same fields.
type sourceJson struct {
	HostProjectId *string `json:"hostProjectId,omitempty"`
	// P0 generates these.
	ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	Region                  *string `json:"region,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type sourceApi = logsource.ItemApi[sourceJson]

func newConnectorModel(item *sourceJson) connectorModel {
	return connectorModel{
		HostProjectId:           types.StringPointerValue(item.HostProjectId),
		ConnectorServiceName:    types.StringPointerValue(item.ConnectorServiceName),
		ConnectorServiceAccount: types.StringPointerValue(item.ConnectorServiceAccount),
		Region:                  types.StringPointerValue(item.Region),
		State:                   types.StringPointerValue(item.State),
	}
}

// P0 generates everything but the host project, so only it is sent.
func (m connectorModel) toJson() *sourceJson {
	return &sourceJson{HostProjectId: m.HostProjectId.ValueStringPointer()}
}

func newInstaller(data *internal.P0ProviderData, component string, getId func(any) *string, fromJson func(context.Context, *diag.Diagnostics, string, any) any, toJson func(any) any) *common.Install {
	return &common.Install{
		Integration:           GcpCloudLoggingKey,
		Component:             component,
		ProviderData:          data,
		GetId:                 getId,
		GetItemJson:           logsource.GetItemJson[sourceJson],
		FromJson:              fromJson,
		ToJson:                toJson,
		DescribeCheckError:    logsource.DescribeCheckError(label, component),
		DescribeWriteNotFound: logsource.DescribeWriteNotFound(label, component),
	}
}

// The attributes of both project resources, before P0 installs the item.
func projectAttributes() map[string]schema.Attribute {
	attributes := connectorAttributes()
	attributes["project_id"] = logsource.FixedAttribute(
		"The Google Cloud project whose Cloud Audit Logs P0 reads",
		projectIdValidator(),
	)
	return attributes
}
