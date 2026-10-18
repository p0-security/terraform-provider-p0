package installjira

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// The `hosting` attribute is the app's `ConnectorHostingWithServiceAccount` install
// element (packages/integrations/jira/shared/src/connector-hosting.ts in the app):
// the app's `ConnectorHosting`, whose `gcp` option also holds the service account the
// connector runs as. P0 names that account after the connector when it creates the
// item, and verifying the item stores the Cloud Run service's URL.
//
// Every address field is `step: "new"`, so P0 refuses a change once the site exists,
// and the shared attribute plans the replacement that works instead. P0's rules for
// the fields are checked in ValidateConfig, in validateHosting, so the fields have no
// validators of their own.
func hostingAttribute() schema.SingleNestedAttribute {
	hosting := installapp.ConnectorHostingWithServiceAccountAttribute(installapp.ConnectorHostingValidators{})
	hosting.MarkdownDescription = "Where the site's connector is deployed, and how P0 addresses it: AWS Lambda or Google Cloud Run."
	return hosting
}

// P0's rules for where a site's connector runs, and its messages for a value that
// breaks one, which it checks when it creates the item (HOSTING_RULES in
// packages/integrations/jira/backend/src/install/site.ts in the app, with the rules in
// packages/integrations/jira/shared/src/aws-hosting.ts, gcp-hosting.ts and
// connector-hosting.ts). The connector's Terraform names the connector's identity
// after the connector, so the name has to work for everything that carries it.
var (
	awsAccountRegex = regexp.MustCompile(`^\d{12}$`)
	// An AWS region, such as us-west-2.
	awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)
	// A region outside AWS's commercial partition: GovCloud, China, the ISO regions or
	// the European Sovereign Cloud. P0 invokes the connector, and points it at the
	// secret, by `arn:aws:` ARNs, so neither can be in one.
	awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)
	// A Lambda function with its ECR repository and its IAM role. ECR is why a name has
	// at least two characters and no two hyphens in a row.
	awsConnectorNameRegex = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	// A Google Cloud project's ID: 6 to 30 lowercase letters, digits and hyphens,
	// starting with a letter and not ending with a hyphen. P0 holds the connector's
	// project to the rule the site's Terraform is rendered with.
	gcpProjectIdRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	// A Google Cloud region, such as us-central1.
	gcpRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+\d+$`)
	// A Cloud Run service and the ID of the service account it runs as: 6 to 30
	// characters. Cloud Run takes any service account ID as a service's name.
	gcpConnectorNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
)

// Lambda and IAM cap a function or role name at 64 characters.
const awsConnectorNameMaxLength = 64

var invalidAwsConnectorName = fmt.Sprintf("Use 2 to %d characters for the connector name: lowercase letters, digits and single hyphens, "+
	"starting with a letter and ending with a letter or digit.", awsConnectorNameMaxLength)

const (
	invalidAwsAccountId  = "Enter the 12-digit ID of the AWS account the connector runs in."
	invalidAwsRegion     = "Enter the AWS region the connector runs in, such as us-west-2."
	unsupportedAwsRegion = "Jira supports only AWS commercial regions."
	invalidGcpProjectId  = "Enter the ID of the Google Cloud project the connector runs in: 6 to 30 lowercase letters, digits and hyphens, " +
		"starting with a letter. Not the project's number or name."
	invalidGcpRegion        = "Enter the Google Cloud region the connector runs in, such as us-central1."
	invalidGcpConnectorName = "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, " +
		"starting with a letter and ending with a letter or digit."
)

// A field of a hosting or vault block, by its attribute name, and its rules.
type fieldRules struct {
	name  string
	rules []rule
}

func awsRegionRules(summary string, invalid string) []rule {
	return []rule{
		{summary: "Invalid AWS region for " + summary, message: invalid, test: awsRegionRegex.MatchString},
		{
			summary: "Unsupported AWS region for " + summary,
			message: unsupportedAwsRegion,
			test:    func(region string) bool { return !awsOtherPartitionRegionRegex.MatchString(region) },
		},
	}
}

// The rules for each hosting type's fields, in the order P0 checks them. A field that
// is missing, or set for the other cloud, is ValidateConnectorHosting's to refuse.
var hostingRules = map[string][]fieldRules{
	installapp.AwsHosting: {
		{name: "account_id", rules: []rule{{summary: "Invalid AWS account ID for the connector", message: invalidAwsAccountId, test: awsAccountRegex.MatchString}}},
		{name: "connector_region", rules: awsRegionRules("the connector", invalidAwsRegion)},
		{name: "connector_name", rules: []rule{{
			summary: "Invalid connector name",
			message: invalidAwsConnectorName,
			test: func(name string) bool {
				return len(name) <= awsConnectorNameMaxLength && awsConnectorNameRegex.MatchString(name)
			},
		}}},
	},
	installapp.GcpHosting: {
		{name: "project_id", rules: []rule{{summary: "Invalid Google Cloud project for the connector", message: invalidGcpProjectId, test: gcpProjectIdRegex.MatchString}}},
		{name: "connector_region", rules: []rule{{summary: "Invalid Google Cloud region for the connector", message: invalidGcpRegion, test: gcpRegionRegex.MatchString}}},
		{name: "connector_name", rules: []rule{{summary: "Invalid connector name", message: invalidGcpConnectorName, test: gcpConnectorNameRegex.MatchString}}},
	},
}

// The hosting's address fields, by attribute name.
func hostingFields(hosting *installapp.ConnectorHostingModel) map[string]types.String {
	return map[string]types.String{
		"account_id":       hosting.AccountId,
		"project_id":       hosting.ProjectId,
		"connector_name":   hosting.ConnectorName,
		"connector_region": hosting.ConnectorRegion,
	}
}

// Rejects what P0 would refuse in where the connector runs: fields that don't match
// the hosting's type, and each field that breaks P0's rule for that type. A hosting
// that is null, or unknown until apply, has nothing to check yet, and neither has one
// whose type isn't known.
func validateHosting(hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	if hosting == nil {
		return
	}
	installapp.ValidateConnectorHosting(hosting, diags)
	if !installapp.IsSet(hosting.Type) {
		return
	}
	validateFields(path.Root("hosting"), hostingFields(hosting), hostingRules[hosting.Type.ValueString()], diags)
}

func validateFields(block path.Path, values map[string]types.String, fields []fieldRules, diags *diag.Diagnostics) {
	for _, field := range fields {
		validateField(block.AtName(field.name), values[field.name], field.rules, diags)
	}
}
