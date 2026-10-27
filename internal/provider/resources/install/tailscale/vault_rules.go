package installtailscale

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// copy of internal/provider/resources/install/auth0/vault_rules.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686).
//
// Where a connector can run on AWS and where an OAuth client's secret can be there: P0's rules
// for the AWS hosting and vault fields, which it checks when it creates the item
// (packages/integrations/tailscale/backend/src/install/validation.ts in the app), and the
// code that checks a block's fields against them. A value that breaks one is one the
// connector's Terraform can't deploy with. The patterns and the messages are P0's.

var (
	// A connector name that works for everything the connector's AWS Terraform names
	// after it: a Lambda function with its ECR repository and IAM role. ECR is why a
	// name has at least two characters and no two hyphens in a row.
	awsConnectorNamePattern = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	awsAccountPattern       = regexp.MustCompile(`^\d{12}$`)
	// An AWS region, such as us-west-2.
	awsRegionPattern = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)
	// A region outside AWS's commercial partition: GovCloud, China, the ISO regions or
	// the European Sovereign Cloud. P0 invokes the connector, and points it at the
	// OAuth client's secret, by `arn:aws:` ARNs, so neither can be in one.
	awsOtherPartitionRegionPattern = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

	// A Secrets Manager secret's name. It admits no whitespace, colon or wildcard, so
	// neither an ARN nor a pattern matches. A Tailscale OAuth client's secret does.
	awsSecretNamePattern = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	// The hyphen and six letters or digits that Secrets Manager adds to a secret's name
	// in its ARN. Secrets Manager reads a partial ARN whose name ends this way as a full
	// ARN, and then finds no secret by it.
	awsSecretNameSuffixPattern = regexp.MustCompile(`-[A-Za-z0-9]{6}$`)
)

// Lambda and IAM cap a function or role name at 64 characters.
const awsConnectorNameMaxLength = 64

// message is P0's, for a value that fails test. summary is the provider's own.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// name is the field's attribute name in its hosting or vault block.
type fieldRules struct {
	name  string
	rules []rule
}

// What a field places: the connector, or the OAuth client's secret. P0's messages say where
// it is, and a summary says what, because Terraform shows a nested attribute's error
// at its resource rather than at the attribute.
type place struct {
	what  string
	where string
}

func awsConnectorNameRules() []rule {
	return []rule{{
		summary: "Invalid connector name",
		message: fmt.Sprintf("Use 2 to %d characters for the connector name: lowercase letters, digits and single hyphens, "+
			"starting with a letter and ending with a letter or digit.", awsConnectorNameMaxLength),
		test: func(name string) bool {
			return awsConnectorNamePattern.MatchString(name) && len(name) <= awsConnectorNameMaxLength
		},
	}}
}

func awsAccountRules(p place) []rule {
	return []rule{{
		summary: "Invalid AWS account ID for " + p.what,
		message: fmt.Sprintf("Enter the 12-digit ID of the AWS account %s.", p.where),
		test:    awsAccountPattern.MatchString,
	}}
}

// P0's rules for the AWS region that holds p, in the order P0 checks them.
func awsRegionRules(p place) []rule {
	return []rule{
		{
			summary: "Invalid AWS region for " + p.what,
			message: fmt.Sprintf("Enter the AWS region %s, such as us-west-2.", p.where),
			test:    awsRegionPattern.MatchString,
		},
		{
			summary: "Unsupported AWS region for " + p.what,
			message: unsupportedAwsPartition,
			test:    func(region string) bool { return !awsOtherPartitionRegionPattern.MatchString(region) },
		},
	}
}

// values holds the block's fields by attribute name.
func validateFields(block path.Path, values map[string]types.String, fields []fieldRules, diags *diag.Diagnostics) {
	for _, field := range fields {
		validateField(block.AtName(field.name), values[field.name], field.rules, diags)
	}
}

// Rejects value, at attribute, with P0's message for the first of rules that it breaks.
// Like P0, it checks the value without the whitespace around it, and then rejects that
// whitespace on its own. P0 trims the strings it stores, so a padded value would read
// back different from the configuration, which Terraform reports as an inconsistent
// result after apply. A null value is left to the checks that a field is set. No error
// repeats the value, because a secret's name may be a pasted OAuth client secret.
func validateField(attribute path.Path, value types.String, rules []rule, diags *diag.Diagnostics) {
	if !isSet(value) {
		return
	}
	trimmed := strings.TrimSpace(value.ValueString())
	for _, r := range rules {
		if !r.test(trimmed) {
			diags.AddAttributeError(attribute, r.summary, r.message)
			return
		}
	}
	if trimmed != value.ValueString() {
		diags.AddAttributeError(
			attribute,
			"Whitespace around a value",
			fmt.Sprintf("P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. "+
				"Remove the whitespace, for example with trimspace().", attribute),
		)
	}
}
