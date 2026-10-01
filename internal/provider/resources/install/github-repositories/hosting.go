package installgithubrepositories

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// Where the connector can run and where the key's secret can be: P0's rules for the
// hosting and vault fields, which it checks when it creates the item
// (packages/integrations/github-repositories-shared/src/hosting.ts in the app). The
// connector's Terraform can't be deployed with a value that breaks one. The patterns
// and the messages are P0's.

var (
	// A connector name that works for everything the connector's Terraform names after
	// it: a Cloud Run service, or a Lambda function with its ECR repository and IAM
	// role. ECR is why a name has at least two characters and no two hyphens in a row.
	connectorNamePattern = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	awsAccountPattern    = regexp.MustCompile(`^\d{12}$`)
	// An AWS region, such as us-west-2.
	awsRegionPattern = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)
	// A region outside AWS's commercial partition: GovCloud, China, the ISO regions or
	// the European Sovereign Cloud. P0 invokes the connector by an `arn:aws:` ARN, so
	// neither the connector nor its secret can be in one.
	awsOtherPartitionRegionPattern = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)
	// A Google Cloud region, such as us-central1.
	gcpRegionPattern = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+\d+$`)
	// A Google Cloud project's ID: 6 to 30 lowercase letters, digits and hyphens,
	// starting with a letter and not ending with a hyphen.
	gcpProjectIdPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
)

// The longest connector name for each hosting: Cloud Run caps a service name at 49
// characters, and Lambda and IAM cap a function or role name at 64.
var connectorNameMaxLength = map[string]int{installapp.AwsHosting: 64, installapp.GcpHosting: 49}

const UnsupportedAwsPartition = "GitHub Repositories doesn't support AWS GovCloud or China regions yet."

// A rule for a value, and P0's message for a value that breaks it.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// A field of a hosting or vault block, by its attribute name, and its rules.
type fieldRules struct {
	name  string
	rules []rule
}

// What a field places: the connector, or the key's secret. P0's messages say where it
// is, and a summary says what, since Terraform shows a nested attribute's error at its
// resource rather than at the attribute.
type place struct {
	what  string
	where string
}

var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the private key's secret", where: "the private key's secret is in"}
)

func connectorNameRules(hostingType string) []rule {
	maxLength := connectorNameMaxLength[hostingType]
	return []rule{{
		summary: "Invalid connector name",
		message: fmt.Sprintf("Use 2 to %d characters for the connector name: lowercase letters, digits and single hyphens, "+
			"starting with a letter and ending with a letter or digit.", maxLength),
		test: func(name string) bool { return connectorNamePattern.MatchString(name) && len(name) <= maxLength },
	}}
}

func awsAccountRules(p place) []rule {
	return []rule{{
		summary: "Invalid AWS account ID for " + p.what,
		message: fmt.Sprintf("Enter the 12-digit ID of the AWS account %s.", p.where),
		test:    awsAccountPattern.MatchString,
	}}
}

func awsRegionRules(p place) []rule {
	return []rule{
		{
			summary: "Invalid AWS region for " + p.what,
			message: fmt.Sprintf("Enter the AWS region %s, such as us-west-2.", p.where),
			test:    awsRegionPattern.MatchString,
		},
		{
			summary: "Unsupported AWS region for " + p.what,
			message: UnsupportedAwsPartition,
			test:    func(region string) bool { return !awsOtherPartitionRegionPattern.MatchString(region) },
		},
	}
}

func gcpProjectRules(p place) []rule {
	return []rule{{
		summary: "Invalid Google Cloud project ID for " + p.what,
		message: fmt.Sprintf("Enter the ID of the Google Cloud project %s.", p.where),
		test:    gcpProjectIdPattern.MatchString,
	}}
}

// The rules for each hosting type's fields, in the order P0 checks them.
var hostingRules = map[string][]fieldRules{
	installapp.AwsHosting: {
		{name: "account_id", rules: awsAccountRules(connectorPlace)},
		{name: "connector_name", rules: connectorNameRules(installapp.AwsHosting)},
		{name: "connector_region", rules: awsRegionRules(connectorPlace)},
	},
	installapp.GcpHosting: {
		{name: "connector_name", rules: connectorNameRules(installapp.GcpHosting)},
		{name: "connector_region", rules: []rule{{
			summary: "Invalid Google Cloud region for the connector",
			message: "Enter the Google Cloud region the connector runs in, such as us-central1.",
			test:    gcpRegionPattern.MatchString,
		}}},
		{name: "project_id", rules: gcpProjectRules(connectorPlace)},
	},
}

// The rules for each vault type's fields, in the order P0 checks them.
var vaultRules = map[string][]fieldRules{
	AwsSecretsManager: {
		{name: "account_id", rules: awsAccountRules(secretPlace)},
		{name: "secrets_region", rules: awsRegionRules(secretPlace)},
	},
	GcpSecretManager: {
		{name: "project_id", rules: gcpProjectRules(secretPlace)},
	},
}

// Rejects each hosting and vault field that P0 refuses, for where the connector runs
// or where the key's secret is. A block of a type that isn't known yet is skipped.
func validateDeployable(vault *vaultModel, hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	if hosting != nil && installapp.IsSet(hosting.Type) {
		validateFields(path.Root("hosting"), hostingFields(hosting), hostingRules[hosting.Type.ValueString()], diags)
	}
	if vault != nil && installapp.IsSet(vault.Type) {
		validateFields(path.Root("vault"), vault.fields(), vaultRules[vault.Type.ValueString()], diags)
	}
}

func validateFields(block path.Path, values map[string]types.String, fields []fieldRules, diags *diag.Diagnostics) {
	for _, field := range fields {
		validateField(block.AtName(field.name), values[field.name], field.rules, diags)
	}
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
