package installlaunchdarkly

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
)

// The LaunchDarkly `connector` item: one deployment of P0's LaunchDarkly connector
// (packages/integrations/launchdarkly/shared/src/components.ts in the app).
// p0_launchdarkly_connector_staged and p0_launchdarkly_connector manage the same
// item, so they share its model, its JSON, its checks and its installers.

const invalidConnectorId = "Invalid LaunchDarkly connector ID"

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
		"id":      itemIdAttribute(idDescription),
		"hosting": hosting,
		"state":   common.StateAttribute,
	}
}

// P0's rule for a Google Cloud project's ID, and its message for one that breaks it
// (packages/integrations/gcloud/shared/src/validator.ts in the app). It is stricter
// than the provider's GcpProjectIdRegex, which takes an underscore.
var gcpProjectIdRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

const invalidGcpProjectId = "Google Cloud project identifiers must consist of alphanumeric characters and hyphens only"

// The checks on the connector's account, project and region. Each pattern also
// refuses whitespace around a value: P0 trims the strings it stores, so such a value
// would read back different from the configuration. The connector's name has a rule
// for each hosting, which a field's validator can't see, so ValidateConfig checks it.
var connectorHostingValidators = installapp.ConnectorHostingValidators{
	AccountId: []validator.String{
		stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
	},
	ProjectId: []validator.String{
		stringvalidator.RegexMatches(gcpProjectIdRegex, invalidGcpProjectId),
	},
	ConnectorRegion: []validator.String{
		stringvalidator.RegexMatches(installapp.ConnectorRegionRegex, "Must be a valid AWS or Google Cloud region"),
	},
}

// P0's rules for the connector's name, and its messages for a name that breaks one
// (packages/integrations/launchdarkly/shared/src/connector-hosting.ts in the app). The
// connector's Terraform names the connector's identity after the connector, so the
// name has to work for everything that carries it.
var (
	// A Lambda function with its ECR repository and its IAM role. ECR is why a name
	// has at least two characters and no two hyphens in a row.
	awsConnectorNameRegex = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	// A Cloud Run service and the ID of the service account it runs as: 6 to 30
	// characters. Cloud Run takes any service account ID as a service's name.
	gcpConnectorNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
)

// Lambda and IAM cap a function or role name at 64 characters.
const awsConnectorNameMaxLength = 64

var invalidAwsConnectorName = fmt.Sprintf("Use 2 to %d characters for the connector name: lowercase letters, digits and single hyphens, "+
	"starting with a letter and ending with a letter or digit.", awsConnectorNameMaxLength)

const invalidGcpConnectorName = "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, " +
	"starting with a letter and ending with a letter or digit."

// The rules, for the resources' descriptions.
var connectorRulesNote = "P0 checks `hosting.connector_name` when it stages the connector, and so does a plan. " +
	"For AWS Lambda: " + invalidAwsConnectorName + " For Google Cloud Run: " + invalidGcpConnectorName

// Reports whether name is one P0 accepts for the hosting, and P0's message if it
// isn't. A hosting with no rule here accepts any name: the type's own validator has
// already refused it.
func checkConnectorName(hostingType string, name string) (bool, string) {
	switch hostingType {
	case installapp.AwsHosting:
		return len(name) <= awsConnectorNameMaxLength && awsConnectorNameRegex.MatchString(name), invalidAwsConnectorName
	case installapp.GcpHosting:
		return gcpConnectorNameRegex.MatchString(name), invalidGcpConnectorName
	default:
		return true, ""
	}
}

// Rejects what P0 would refuse when it stages the connector, at plan time and in P0's
// words: a connector name that breaks P0's rule, and hosting fields that don't match
// the hosting's type. The attributes' own validators check the rest. A value that
// Terraform only knows at apply time is skipped while planning, and checked when
// Terraform validates the configuration again before it applies.
func validateConnector(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	hosting := installapp.ConnectorHostingWithServiceAccountFromConfig(ctx, config, diags)
	// A hosting that is null, or unknown until apply, has nothing to check yet.
	if diags.HasError() || hosting == nil {
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
	name := hosting.ConnectorName.ValueString()
	if ok, message := checkConnectorName(hosting.Type.ValueString(), name); !ok {
		diags.AddAttributeError(
			path.Root("hosting").AtName("connector_name"),
			"Invalid LaunchDarkly connector name",
			fmt.Sprintf("%s Got: %q", message, name),
		)
	}
}

// The installer that stages, verifies and configures the item, and rolls it back.
func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:        LaunchDarklyKey,
		Component:          installresources.Connector,
		ProviderData:       data,
		GetId:              connectorId,
		GetItemJson:        connectorItemJson,
		FromJson:           connectorFromJson(missingServiceAccountOnWrite),
		ToJson:             connectorToJson,
		DescribeCheckError: describeConnectorCheckError,
	}
}

// The installer that reads the item. It differs from the other only in what it says
// when P0 returns a Google Cloud connector without its service account: a refresh
// fails in place of blanking the account, and says how to plan in the meantime. A
// connector that P0 no longer has is dropped from state, as other resources do, and
// the next apply stages it again.
func newConnectorReader(data *internal.P0ProviderData) *common.Install {
	reader := newConnectorInstaller(data)
	reader.FromJson = connectorFromJson(missingServiceAccountOnRead)
	return reader
}

// P0 checks the connector where the customer deployed it, so a failed check usually
// means a problem in the customer's setup, which P0's message describes. So does its
// refusal to verify a connector whose hosting isn't the staged one. The wording
// doesn't presume which it is, and leaves that to P0's message.
func describeConnectorCheckError(id string, err error) (string, string) {
	return "LaunchDarkly connector install check failed",
		fmt.Sprintf("P0 rejected the install check for the LaunchDarkly connector %q:\n\n%s", id, err)
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
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *connectorJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*connectorApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

// What follows the reason in the diagnostic for a Google Cloud connector that P0
// returned without its service account.
const (
	missingServiceAccountOnWrite = "Contact " + supportEmail + "."
	// A warning would leave the account null in state, and Terraform would then plan
	// the Terraform that deploys the connector from the null: a service account ID it
	// can't compute, or, where an earlier value is kept, one that replaces the
	// account, which every LaunchDarkly account's secret grant names. Failing plans
	// nothing.
	missingServiceAccountOnRead = "Contact " + supportEmail + ". Until P0 returns the account, plan and apply other changes with `-refresh=false`, " +
		"or remove the connector from Terraform state with `terraform state rm` and import it again once P0 returns it."
)

func connectorFromJson(missingServiceAccount string) func(context.Context, *diag.Diagnostics, string, any) any {
	return func(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
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
				fmt.Sprintf("The LaunchDarkly connector %s carries no hosting configuration. "+
					"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
			)
			return nil
		}

		// On Google Cloud the connector must run as the service account P0 generates
		// when it stages the item, so it can't be deployed without one.
		if jsonv.Hosting.Type == installapp.GcpHosting && (jsonv.Hosting.ConnectorServiceAccount == nil || *jsonv.Hosting.ConnectorServiceAccount == "") {
			diags.AddError(
				"Bad API response",
				fmt.Sprintf("The LaunchDarkly connector %s is hosted on Google Cloud, but P0 returned no service account for it. "+
					"The connector must run as the account that P0 generates when it stages the connector, so it can't be deployed without one. %s",
					id, missingServiceAccount),
			)
			return nil
		}

		return &connectorModel{
			Id:      types.StringValue(id),
			Hosting: installapp.ConnectorHostingWithServiceAccountFromJson(jsonv.Hosting),
			State:   types.StringPointerValue(jsonv.State),
		}
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
