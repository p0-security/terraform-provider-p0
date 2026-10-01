package installgithubrepositories

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// P0's messages, as its own tests spell them out.
const (
	awsNameRule      = "Use 2 to 64 characters for the connector name: lowercase letters, digits and single hyphens, starting with a letter and ending with a letter or digit."
	awsAccountRule   = "Enter the 12-digit ID of the AWS account the connector runs in."
	awsRegionRule    = "Enter the AWS region the connector runs in, such as us-west-2."
	vaultAccountRule = "Enter the 12-digit ID of the AWS account the private key's secret is in."
	vaultRegionRule  = "Enter the AWS region the private key's secret is in, such as us-west-2."
	unsupported      = "GitHub Repositories doesn't support AWS GovCloud or China regions yet."
	invalidOrg       = "Enter the organization's GitHub login, as in github.com/<login>"
	invalidAppId     = "The GitHub App ID is a number. Find it on the app's settings page."
)

// Each error as "<path>: <detail>".
func errorDetails(diags diag.Diagnostics) []string {
	got := []string{}
	for _, d := range diags.Errors() {
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			got = append(got, "<no path>: "+d.Detail())
			continue
		}
		got = append(got, fmt.Sprintf("%s: %s", withPath.Path(), d.Detail()))
	}
	sort.Strings(got)
	return got
}

// P0's own cases for its hosting and vault rules
// (packages/integrations/github-repositories-shared/src/__tests__/hosting.test.ts in the
// app) for AWS, plus the region edges it settled on, run through ValidateConfig: each refused
// value gets one error, at its attribute, with P0's message.
func TestRepositoryAccessValidateConfigHostingAndVault(t *testing.T) {
	awsName := func(name string) func(tftypes.Object) tftypes.Value {
		return with(awsHostingFields, "connector_name", str(name))
	}
	awsRegion := func(region string) func(tftypes.Object) tftypes.Value {
		return with(awsHostingFields, "connector_region", str(region))
	}

	cases := []struct {
		name    string
		vault   func(tftypes.Object) tftypes.Value
		hosting func(tftypes.Object) tftypes.Value
		want    []string
	}{
		{name: "aws", vault: awsVault, hosting: awsHosting},
		{name: "a longest Lambda name", vault: awsVault, hosting: awsName("p" + strings.Repeat("0", 63))},
		{name: "a name with capitals", vault: awsVault, hosting: awsName("P0-GitHub"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name of one character", vault: awsVault, hosting: awsName("p"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with two hyphens in a row", vault: awsVault, hosting: awsName("p0--github"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with a trailing hyphen", vault: awsVault, hosting: awsName("p0-github-"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with a leading digit", vault: awsVault, hosting: awsName("0p-github"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with an underscore", vault: awsVault, hosting: awsName("p0_github"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a Lambda name over 64 characters", vault: awsVault, hosting: awsName("p" + strings.Repeat("0", 64)), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "an 11-digit account", vault: awsVault, hosting: with(awsHostingFields, "account_id", str("11111111111")), want: []string{"hosting.account_id: " + awsAccountRule}},
		{name: "an AWS region without its number", vault: awsVault, hosting: awsRegion("us-west"), want: []string{"hosting.connector_region: " + awsRegionRule}},
		{name: "a Google Cloud region on AWS", vault: awsVault, hosting: awsRegion("us-central1"), want: []string{"hosting.connector_region: " + awsRegionRule}},
		{name: "GovCloud", vault: awsVault, hosting: awsRegion("us-gov-west-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "China", vault: awsVault, hosting: awsRegion("cn-north-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ISO", vault: awsVault, hosting: awsRegion("us-iso-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ISO-B", vault: awsVault, hosting: awsRegion("us-isob-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ISO-E", vault: awsVault, hosting: awsRegion("eu-isoe-west-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "the European Sovereign Cloud", vault: awsVault, hosting: awsRegion("eusc-de-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ap-southeast-7", vault: awsVault, hosting: awsRegion("ap-southeast-7")},
		{name: "mx-central-1", vault: awsVault, hosting: awsRegion("mx-central-1")},
		{name: "il-central-1", vault: awsVault, hosting: awsRegion("il-central-1")},
		{name: "ca-west-1", vault: awsVault, hosting: awsRegion("ca-west-1")},
		{name: "a vault account that isn't one", vault: with(awsVaultFields, "account_id", str("acme")), hosting: awsHosting, want: []string{"vault.account_id: " + vaultAccountRule}},
		{name: "a vault region without its number", vault: with(awsVaultFields, "secrets_region", str("us-west")), hosting: awsHosting, want: []string{"vault.secrets_region: " + vaultRegionRule}},
		{name: "a vault region in China", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), hosting: awsHosting, want: []string{"vault.secrets_region: " + unsupported}},
		{name: "a vault region in ISO-E", vault: with(awsVaultFields, "secrets_region", str("eu-isoe-west-1")), hosting: awsHosting, want: []string{"vault.secrets_region: " + unsupported}},
		// P0 checks a value trimmed, and stores it trimmed, so a padded value that's valid
		// otherwise is refused for its whitespace alone.
		{
			name:    "a padded region",
			vault:   awsVault,
			hosting: awsRegion(" us-west-2 "),
			want: []string{"hosting.connector_region: P0 removes the whitespace around 'hosting.connector_region', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "a padded region in China", vault: awsVault, hosting: awsRegion(" cn-north-1"), want: []string{"hosting.connector_region: " + unsupported}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateConfig(t, c.vault, c.hosting, nil))
			want := append([]string{}, c.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// The org and the App ID are refused in P0's words.
func TestRepositoryAccessValidateConfigOrgAndAppId(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]tftypes.Value
		want      []string
	}{
		{name: "an org with an underscore", overrides: map[string]tftypes.Value{"org": str("acme_corp")}, want: []string{"org: " + invalidOrg}},
		{name: "an org of 40 characters", overrides: map[string]tftypes.Value{"org": str(strings.Repeat("a", 40))}, want: []string{"org: " + invalidOrg}},
		{name: "an org of 39 characters", overrides: map[string]tftypes.Value{"org": str(strings.Repeat("a", 39))}},
		{name: "an App ID that isn't a number", overrides: map[string]tftypes.Value{"app_id": str("Iv1.abc123")}, want: []string{"app_id: " + invalidAppId}},
		{name: "a negative App ID", overrides: map[string]tftypes.Value{"app_id": str("-1")}, want: []string{"app_id: " + invalidAppId}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateConfig(t, awsVault, awsHosting, c.overrides))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors = %v; want %v", got, want)
			}
		})
	}
}

// ValidateConfig checks the hosting's address fields against P0's rules, so the
// attribute's own validators would only repeat its errors in other words.
func TestRepositoryAccessHostingFieldsHaveNoValidators(t *testing.T) {
	hosting, ok := repositoryAccessSchema(t).Attributes["hosting"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("hosting is not a nested attribute")
	}
	for _, name := range []string{"account_id", "connector_name", "connector_region"} {
		field, ok := hosting.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("hosting.%s is not a string attribute", name)
		}
		if len(field.Validators) != 0 {
			t.Errorf("hosting.%s has %d validators; want none", name, len(field.Validators))
		}
	}
}

// Every field a rule names is one the block's model has. A misspelt name would read a
// null value, which skips the check silently.
func TestRuleFieldNames(t *testing.T) {
	hostingNames := hostingFields(awsModel().Hosting)
	vaultNames := awsModel().Vault.fields()
	for hostingType, fields := range hostingRules {
		for _, field := range fields {
			if _, ok := hostingNames[field.name]; !ok {
				t.Errorf("hosting rule for %s names %q, which the hosting doesn't have", hostingType, field.name)
			}
		}
	}
	for vaultType, fields := range vaultRules {
		for _, field := range fields {
			if _, ok := vaultNames[field.name]; !ok {
				t.Errorf("vault rule for %s names %q, which the vault doesn't have", vaultType, field.name)
			}
		}
	}
}

// Terraform shows a nested attribute's error at its resource, and P0's message for a
// region outside AWS's commercial partition doesn't say whose region it is, so the
// summary does.
func TestRepositoryAccessValidateConfigPartitionSummaries(t *testing.T) {
	vault := with(awsVaultFields, "secrets_region", str("cn-north-1"))
	hosting := with(awsHostingFields, "connector_region", str("us-gov-west-1"))
	got := []string{}
	for _, d := range validateConfig(t, vault, hosting, nil).Errors() {
		got = append(got, d.Summary()+": "+d.Detail())
	}
	sort.Strings(got)
	want := []string{
		"Unsupported AWS region for the connector: " + unsupported,
		"Unsupported AWS region for the private key's secret: " + unsupported,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("errors = %v; want %v", got, want)
	}
}

// The hosting's type takes only `aws` for now.
func TestHostingTypeValidator(t *testing.T) {
	hostingType, ok := hostingAttribute().Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("hosting.type is not a string attribute")
	}
	for value, want := range map[string]bool{"aws": true, "gcp": false, "": false} {
		if got := accepts(t, hostingType, value); got != want {
			t.Errorf("hosting.type accepts %q = %v; want %v", value, got, want)
		}
	}
}
