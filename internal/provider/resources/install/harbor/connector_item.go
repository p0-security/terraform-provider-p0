package installharbor

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// The Harbor `connector` item: one deployment of P0's Harbor connector
// (packages/integrations/harbor/shared/src/components.ts in the app).
// p0_harbor_connector_staged and p0_harbor_connector manage the same item, so they
// share its model, its JSON, its checks and its installer.

const invalidConnectorId = "Invalid Harbor connector ID"

// The rules connectorNameRules holds, for the resources' descriptions.
const connectorRulesNote = "P0 checks `hosting.connector_name` when it stages the connector, and so does a plan. " +
	"For AWS Lambda, use 2 to 64 characters: lowercase letters, digits and single hyphens, starting with a letter and ending with a letter or digit. " +
	"For Google Cloud Run, use 6 to 30 characters: lowercase letters, digits and hyphens, starting with a letter and ending with a letter or digit."

type connectorModel struct {
	Id      types.String                                        `tfsdk:"id"`
	Hosting *installapp.ConnectorHostingWithServiceAccountModel `tfsdk:"hosting"`
	State   types.String                                        `tfsdk:"state"`
}

// The item, as P0 stores it.
type connectorJson struct {
	Hosting *installapp.ConnectorHostingJson `json:"hosting,omitempty"`
	State   *string                          `json:"state,omitempty"`
}

type connectorApi struct {
	Item *connectorJson `json:"item"`
}

// The resources' attributes. P0 fixes the ID and the hosting when it creates the item
// and refuses a change to either, so each requires replacement.
func connectorAttributes(idDescription string, hostingDescription string) map[string]schema.Attribute {
	hosting := installapp.ConnectorHostingWithServiceAccountAttribute(connectorHostingValidators)
	hosting.MarkdownDescription = hostingDescription
	return map[string]schema.Attribute{
		"id":      installvaultedcredential.IdAttribute(idDescription),
		"hosting": hosting,
		"state":   common.StateAttribute,
	}
}

// The checks on the connector's account, project and region, which p0_app makes on the
// same fields. Each pattern also refuses whitespace around a value: P0 trims the
// strings it stores, so such a value would read back different from the configuration.
// The connector's name has a rule for each hosting, which a field's validator can't
// see, so ValidateConfig checks it.
var connectorHostingValidators = installapp.ConnectorHostingValidators{
	AccountId: []validator.String{
		stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
	},
	ProjectId: []validator.String{
		stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
	},
	ConnectorRegion: []validator.String{
		stringvalidator.RegexMatches(installapp.ConnectorRegionRegex, "Must be a valid AWS or Google Cloud region"),
	},
}

// P0's rules for the connector's name, and its message for a name that breaks one
// (packages/integrations/app/shared/src/hosting.ts in the app). The connector's
// Terraform names the connector's identity after the connector, so the name has to
// work for everything that carries it.
var (
	// A Lambda function with its ECR repository and its IAM role. ECR is why a name
	// has at least two characters and no two hyphens in a row.
	awsConnectorNamePattern = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	// A Cloud Run service and the ID of the service account it runs as: 6 to 30
	// characters. Cloud Run takes any service account ID as a service's name.
	gcpConnectorNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
)

// Lambda and IAM cap a function or role name at 64 characters.
const awsConnectorNameMaxLength = 64

type connectorNameRule struct {
	accepts func(name string) bool
	message string
}

var connectorNameRules = map[string]connectorNameRule{
	installapp.AwsHosting: {
		accepts: func(name string) bool {
			return len(name) <= awsConnectorNameMaxLength && awsConnectorNamePattern.MatchString(name)
		},
		message: "Use 2 to 64 characters for the connector name: lowercase letters, digits and single hyphens, " +
			"starting with a letter and ending with a letter or digit.",
	},
	installapp.GcpHosting: {
		accepts: gcpConnectorNamePattern.MatchString,
		message: "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, " +
			"starting with a letter and ending with a letter or digit.",
	},
}

// Rejects what P0 would refuse when it stages the connector, at plan time and in P0's
// words: an ID or a connector name that breaks P0's rule, and hosting fields that
// don't match the hosting's type. A value that Terraform only knows at apply time is
// skipped while planning, and checked when Terraform validates the configuration again
// before it applies.
func validateConnector(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	var id types.String
	diags.Append(config.GetAttribute(ctx, path.Root("id"), &id)...)
	hosting := installapp.ConnectorHostingWithServiceAccountFromConfig(ctx, config, diags)
	if diags.HasError() {
		return
	}

	validateItemId(id, invalidConnectorId, diags)
	// A hosting that is null, or unknown until apply, has nothing to check yet.
	if hosting == nil {
		return
	}
	installapp.ValidateConnectorHosting(&hosting.ConnectorHostingModel, diags)
	validateConnectorName(&hosting.ConnectorHostingModel, diags)
}

// P0 checks the name without the whitespace around it, and stores it that way. The
// name is checked as it is here, so whitespace around it fails the rule: the name P0
// stored would read back different from the configuration. The message quotes the
// name, so that such whitespace shows.
func validateConnectorName(hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	if !installapp.IsSet(hosting.Type) || !installapp.IsSet(hosting.ConnectorName) {
		return
	}
	// The type's own validator has already rejected a hosting with no rule here.
	rule, ok := connectorNameRules[hosting.Type.ValueString()]
	if !ok {
		return
	}
	name := hosting.ConnectorName.ValueString()
	if !rule.accepts(name) {
		diags.AddAttributeError(
			path.Root("hosting").AtName("connector_name"),
			"Invalid Harbor connector name",
			fmt.Sprintf("%s Got: %q", rule.message, name),
		)
	}
}

func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:           HarborKey,
		Component:             installresources.Connector,
		ProviderData:          data,
		GetId:                 connectorId,
		GetItemJson:           connectorItemJson,
		FromJson:              connectorFromJson,
		ToJson:                connectorToJson,
		DescribeCheckError:    describeConnectorCheckError,
		DescribeWriteNotFound: describeUnavailable,
	}
}

// P0 checks the connector where the customer deployed it, so a failed check usually
// means a problem in the customer's setup, which P0's message describes. So does its
// refusal to verify a connector whose hosting isn't the staged one. The wording
// doesn't presume which it is, and leaves that to P0's message.
func describeConnectorCheckError(id string, err error) (string, string) {
	return "Harbor connector install check failed",
		fmt.Sprintf("P0 rejected the install check for the Harbor connector %q:\n\n%s", id, err)
}

func connectorId(data any) *string {
	model, ok := data.(*connectorModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func connectorItemJson(json any) any {
	inner, ok := json.(*connectorApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func connectorFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*connectorJson)
	if !ok {
		return nil
	}

	// hosting is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Harbor connector %s carries no hosting configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// On Google Cloud the connector must run as the service account P0 generates when
	// it stages the item. A null one would reach the Terraform that deploys the
	// connector, and fail there with an error about a null value.
	if jsonv.Hosting.Type == installapp.GcpHosting && (jsonv.Hosting.ConnectorServiceAccount == nil || *jsonv.Hosting.ConnectorServiceAccount == "") {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Harbor connector %s is hosted on Google Cloud, but P0 returned no service account for it. "+
				"The connector must run as the account that P0 generates when it stages the connector, so it can't be deployed without one. "+
				"Contact support@p0.dev.", id),
		)
		return nil
	}

	return &connectorModel{
		Id:      types.StringValue(id),
		Hosting: installapp.ConnectorHostingWithServiceAccountFromJson(jsonv.Hosting),
		State:   types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: where the connector runs. P0 sets the item's other
// fields, so the hosting's ToJson leaves them out.
func connectorToJson(data any) any {
	datav, ok := data.(*connectorModel)
	if !ok {
		return nil
	}

	json := connectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.ToJson()
	}
	return &json
}
