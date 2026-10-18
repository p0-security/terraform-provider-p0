package installclickhouse

// copy of internal/provider/resources/install/auth0/tenant_test.go (#245), itself a
// copy of Docker Hub's organization_test.go, renamed for ClickHouse Cloud (INT-695),
// without the client ID's cases and with the four fields P0 reads when it validates
// the install.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// An organization whose API key is in one cloud's secret manager, as Terraform and as
// P0's API hold it.
type organizationCloud struct {
	name  string
	vault func() *vaultModel
	// The name of the secret that holds the API key.
	secret string
	// The body of every request that carries the organization: its connector, its vault
	// and its secret's name.
	body string
}

var awsOrganization = organizationCloud{
	name:   "aws",
	vault:  awsVault,
	secret: "clickhouse/acme/api-key",
	body: `{"connector":"prod",` +
		`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"apiKeySecretName":"clickhouse/acme/api-key"}`,
}

var gcpOrganization = organizationCloud{
	name:   "gcp",
	vault:  gcpVault,
	secret: "clickhouse-acme-api-key",
	body: `{"connector":"prod",` +
		`"vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"apiKeySecretName":"clickhouse-acme-api-key"}`,
}

var organizationClouds = []organizationCloud{awsOrganization, gcpOrganization}

// The fields P0 reads when it validates the install, in the item's JSON. P0 stores
// each as "" when it stages the item, until a verify reads them.
func validatedFields(staged bool) map[string]string {
	if staged {
		return map[string]string{"organizationName": "", "apiKeyId": "", "allowedDomains": "", "serviceStatus": ""}
	}
	return map[string]string{
		"organizationName": fakeOrganizationName,
		"apiKeyId":         fakeApiKeyId,
		"allowedDomains":   fakeAllowedDomains,
		"serviceStatus":    fakeServiceStatus,
	}
}

// The organization at state, with what P0 reads once it has validated the install.
func (c organizationCloud) organization(state string) *clickhouseOrganizationModel {
	validated := validatedFields(state == common.StateStage)
	return &clickhouseOrganizationModel{
		Id:               types.StringValue(testOrganization),
		Connector:        types.StringValue("prod"),
		Vault:            c.vault(),
		ApiKeySecretName: types.StringValue(c.secret),
		OrganizationName: types.StringValue(validated["organizationName"]),
		ApiKeyId:         types.StringValue(validated["apiKeyId"]),
		AllowedDomains:   types.StringValue(validated["allowedDomains"]),
		ServiceStatus:    types.StringValue(validated["serviceStatus"]),
		State:            types.StringValue(state),
	}
}

// The plan for a new organization: everything P0 assigns is unknown.
func (c organizationCloud) planned() *clickhouseOrganizationModel {
	planned := c.organization(common.StateStage)
	planned.OrganizationName = types.StringUnknown()
	planned.ApiKeyId = types.StringUnknown()
	planned.AllowedDomains = types.StringUnknown()
	planned.ServiceStatus = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The item as P0's API answers at state.
func (c organizationCloud) response(t *testing.T, state string) string {
	t.Helper()
	item := map[string]any{}
	if err := json.Unmarshal([]byte(c.body), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for name, value := range validatedFields(state == common.StateStage) {
		item[name] = value
	}
	item["state"] = state
	return marshal(t, item)
}

// The PUT that stages the item, and verify and configure, carry the organization's
// connector, vault and secret name, in the field names of the app's install schema.
// `vault` is the shared Vault element, in which "install" names the account or the
// project. None carries what P0 assigns.
func TestOrganizationRequestJson(t *testing.T) {
	for _, c := range organizationClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&ClickHouseOrganization{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&ClickHouseOrganization{}).toJson(c.organization(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed organization =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in: the state and, once it has validated the install, the organization's name, the
// key's ID, the allowed domains and the services' status.
func TestOrganizationFromJson(t *testing.T) {
	for _, c := range organizationClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item clickhouseOrganizationJson
				if err := json.Unmarshal([]byte(c.response(t, state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&ClickHouseOrganization{}).fromJson(context.Background(), &diags, testOrganization, &item).(*clickhouseOrganizationModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.organization(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewClickHouseOrganization(), got)
			})
		}
	}
}

func TestOrganizationFromJsonRequiresVault(t *testing.T) {
	var diags diag.Diagnostics
	got := (&ClickHouseOrganization{}).fromJson(context.Background(), &diags, testOrganization, &clickhouseOrganizationJson{})
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
		t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
	}
}

// A vault of a type this provider doesn't know is refused rather than read into the
// wrong fields.
func TestOrganizationFromJsonRequiresAKnownVault(t *testing.T) {
	item := clickhouseOrganizationJson{Vault: &vaultJson{Type: "azure-kv", Install: "acme"}}

	var diags diag.Diagnostics
	got := (&ClickHouseOrganization{}).fromJson(context.Background(), &diags, testOrganization, &item)
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported ClickHouse Cloud organization" {
		t.Errorf("fromJson = %+v, %v; want nil and an unsupported organization error", got, diags)
	}
	// The error names the organization, its vault and the vaults this provider takes.
	const want = `The ClickHouse Cloud organization 4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f has a "azure-kv" vault. This provider supports only aws-sm and gcp-sm.`
	for _, d := range diags.Errors() {
		if !strings.HasPrefix(d.Detail(), want) {
			t.Errorf("error = %q; want it to start with %q", d.Detail(), want)
		}
	}
}

// P0 fixes the connector, the vault and the secret's name when it creates the item,
// so changing any of them, or the ID, replaces the installation. What P0 assigns is
// read-only.
func TestOrganizationSchema(t *testing.T) {
	s := schemaOf(t, NewClickHouseOrganization())
	vault := nestedAttributes(t, s, "vault")
	for field, attribute := range map[string]schema.Attribute{
		"id":                   s.Attributes["id"],
		"connector":            s.Attributes["connector"],
		"api_key_secret_name":  s.Attributes["api_key_secret_name"],
		"vault.type":           vault["type"],
		"vault.account_id":     vault["account_id"],
		"vault.project_id":     vault["project_id"],
		"vault.secrets_region": vault["secrets_region"],
	} {
		if !requiresReplace(t, attribute) {
			t.Errorf("%s doesn't require replacement", field)
		}
	}

	for _, name := range []string{"organization_name", "api_key_id", "allowed_domains", "service_status", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
}

// ModifyPlan leaves unknown each attribute P0 sets when it validates the install, and
// no other: each is a read-only attribute of the schema.
func TestValidatedAttributes(t *testing.T) {
	s := schemaOf(t, NewClickHouseOrganization())
	want := map[string]bool{}
	for name, attribute := range s.Attributes {
		if attribute.IsComputed() && name != "state" {
			want[name] = true
		}
	}
	got := map[string]bool{}
	for _, attribute := range validatedAttributes {
		got[attribute.String()] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("validated attributes = %v; want the schema's read-only attributes but state, %v", got, want)
	}
}

// P0's messages, as its own tests spell them out.
const (
	vaultAccountRule  = "Enter the 12-digit ID of the AWS account the API key's secret is in."
	vaultRegionRule   = "Enter the AWS region the API key's secret is in, such as us-west-2."
	vaultProjectRule  = "Enter the ID of the Google Cloud project the API key's secret is in."
	secretNameRule    = "That doesn't look like a secret name. Enter the name of the secret that holds the API key, not its ARN, its full resource name or the key itself."
	secretSuffixRule  = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule = "Enter the name of the secret that holds, or will hold, the API key."
)

var (
	awsVaultFields = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpVaultFields = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}
)

// Runs the organization's ValidateConfig against a configuration with the given vault
// and secret name.
func validateOrganization(t *testing.T, vault func(tftypes.Object) tftypes.Value, secret tftypes.Value) diag.Diagnostics {
	t.Helper()
	blocks := map[string]func(tftypes.Object) tftypes.Value{"vault": vault}
	values := map[string]tftypes.Value{"id": str(testOrganization), "connector": str("prod"), "api_key_secret_name": secret}
	return validateConfig(t, &ClickHouseOrganization{}, blocks, values)
}

// P0's own cases for where the API key is
// (packages/integrations/clickhouse/backend/src/install/__tests__/organization.test.ts
// in the app), plus the edges of each rule and the fields each vault takes, run through
// ValidateConfig: each refused value gets one error, at its attribute, with P0's
// message. Each check skips a value Terraform doesn't know yet.
func TestOrganizationValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:clickhouse/acme/api-key"
	awsVault, gcpVault := objectOf(awsVaultFields), objectOf(gcpVaultFields)
	awsSecret, gcpSecret := str(awsOrganization.secret), str(gcpOrganization.secret)

	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret tftypes.Value
		want   []string
	}{
		{name: "aws", vault: awsVault, secret: awsSecret},
		{name: "gcp", vault: gcpVault, secret: gcpSecret},
		{name: "unknown vault", vault: unknownObject, secret: awsSecret},
		{name: "unknown vault type", vault: objectOf(map[string]tftypes.Value{"type": unknownString, "account_id": str("acme")}), secret: str(arn)},
		{name: "unknown secret name", vault: awsVault, secret: unknownString},
		{name: "unknown secrets region", vault: with(awsVaultFields, "secrets_region", unknownString), secret: awsSecret},
		// The same cloud as the connector, but not the same account or project.
		{name: "a secret in another account", vault: with(awsVaultFields, "account_id", str("210987654321")), secret: awsSecret},

		// Where the secret is.
		{name: "an account that isn't an AWS account ID", vault: with(awsVaultFields, "account_id", str("acme-secrets")), secret: awsSecret, want: []string{"vault.account_id: " + vaultAccountRule}},
		{name: "a Google Cloud region on AWS", vault: with(awsVaultFields, "secrets_region", str("us-west1")), secret: awsSecret, want: []string{"vault.secrets_region: " + vaultRegionRule}},
		{name: "a GovCloud region", vault: with(awsVaultFields, "secrets_region", str("us-gov-west-1")), secret: awsSecret, want: []string{"vault.secrets_region: " + unsupported}},
		{name: "a project that isn't a project ID", vault: with(gcpVaultFields, "project_id", str("123456789012")), secret: gcpSecret, want: []string{"vault.project_id: " + vaultProjectRule}},

		// The fields each vault takes.
		{name: "aws without an account", vault: without(awsVaultFields, "account_id"), secret: awsSecret, want: []string{`vault.account_id: 'vault.account_id' is required when 'vault.type' is "aws-sm".`}},
		{name: "aws without a region", vault: without(awsVaultFields, "secrets_region"), secret: awsSecret, want: []string{`vault.secrets_region: 'vault.secrets_region' is required when 'vault.type' is "aws-sm".`}},
		{
			name:   "aws with a project",
			vault:  with(awsVaultFields, "project_id", str("acme-secrets")),
			secret: awsSecret,
			want:   []string{`vault.project_id: 'vault.project_id' may only be set when 'vault.type' is "gcp-sm".`},
		},
		{name: "gcp without a project", vault: without(gcpVaultFields, "project_id"), secret: gcpSecret, want: []string{`vault.project_id: 'vault.project_id' is required when 'vault.type' is "gcp-sm".`}},
		{
			name:   "gcp with an account",
			vault:  with(gcpVaultFields, "account_id", str("123456789012")),
			secret: gcpSecret,
			want:   []string{`vault.account_id: 'vault.account_id' may only be set when 'vault.type' is "aws-sm".`},
		},
		{
			name:   "gcp with a region",
			vault:  with(gcpVaultFields, "secrets_region", str("us-west-2")),
			secret: gcpSecret,
			want:   []string{`vault.secrets_region: 'vault.secrets_region' may only be set when 'vault.type' is "aws-sm".`},
		},

		// The secret's name, which each secret manager has its own rule for.
		{name: "an empty name", vault: awsVault, secret: str(""), want: []string{"api_key_secret_name: " + missingSecretRule}},
		{name: "an empty name with an unknown vault", vault: unknownObject, secret: str(""), want: []string{"api_key_secret_name: " + missingSecretRule}},
		// P0 trims the name whatever the vault, so this doesn't wait for the vault's type.
		{
			name:   "a padded name with an unknown vault type",
			vault:  objectOf(map[string]tftypes.Value{"type": unknownString}),
			secret: str(" clickhouse/api-key"),
			want: []string{"api_key_secret_name: P0 removes the whitespace around 'api_key_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an ARN", vault: awsVault, secret: str(arn + "-AbCdEf"), want: []string{"api_key_secret_name: " + secretNameRule}},
		{name: "a name with a space", vault: awsVault, secret: str("clickhouse api-key"), want: []string{"api_key_secret_name: " + secretNameRule}},
		{name: "a wildcard", vault: awsVault, secret: str("clickhouse/*"), want: []string{"api_key_secret_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", vault: awsVault, secret: str("clickhouse/key-AbCdEf"), want: []string{"api_key_secret_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", vault: gcpVault, secret: str("projects/acme-secrets/secrets/clickhouse-api-key"), want: []string{"api_key_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", vault: gcpVault, secret: awsSecret, want: []string{"api_key_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", vault: gcpVault, secret: str("clickhouse.api-key"), want: []string{"api_key_secret_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", vault: gcpVault, secret: str("clickhouse-key-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", vault: awsVault, secret: str(" clickhouse/key-AbCdEf\n"), want: []string{"api_key_secret_name: " + secretSuffixRule}},
		{
			name:   "a name with a trailing newline",
			vault:  awsVault,
			secret: str("clickhouse/api-key\n"),
			want: []string{"api_key_secret_name: P0 removes the whitespace around 'api_key_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateOrganization(t, c.vault, c.secret))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Terraform shows a nested attribute's error at its resource, so each summary says what
// the value is for. No error repeats the secret's name, because a customer may have
// pasted the API key in its place.
func TestOrganizationValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret string
		want   []string
	}{
		{name: "an ARN", vault: objectOf(awsVaultFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid secret name for the API key"}},
		{name: "a full resource name", vault: objectOf(gcpVaultFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid secret name for the API key"}},
		{name: "a suffixed name", vault: objectOf(awsVaultFields), secret: "acme/token-Zx9Qw2", want: []string{"Secret name for the API key ends like an ARN suffix"}},
		{name: "a padded name", vault: objectOf(awsVaultFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", vault: objectOf(awsVaultFields), secret: "", want: []string{"Missing secret name for the API key"}},
		{name: "vault account", vault: with(awsVaultFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the API key's secret"}},
		{name: "vault region", vault: with(awsVaultFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the API key's secret"}},
		{name: "vault partition", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the API key's secret"}},
		{name: "vault project", vault: with(gcpVaultFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the API key's secret"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateOrganization(t, c.vault, str(c.secret))
			if got := errorSummaries(diags); !reflect.DeepEqual(got, c.want) {
				t.Errorf("errors = %v; want %v", got, c.want)
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Summary()+d.Detail(), "acme/token") || strings.Contains(d.Summary()+d.Detail(), "acme-token") {
					t.Errorf("error %v repeats the value", d)
				}
			}
		})
	}
}

// Every field a rule names is one the vault has. A misspelt name would read a null
// value, which skips the check silently.
func TestVaultRuleFieldNames(t *testing.T) {
	names := awsOrganization.vault().fields()
	attributes := nestedAttributes(t, schemaOf(t, NewClickHouseOrganization()), "vault")
	for vaultType, fields := range vaultRules {
		for _, field := range fields {
			if _, ok := names[field.name]; !ok {
				t.Errorf("vault rule for %s names %q, which the vault doesn't have", vaultType, field.name)
			}
			if _, ok := attributes[field.name]; !ok {
				t.Errorf("vault rule for %s names %q, which the attribute doesn't have", vaultType, field.name)
			}
		}
	}
}

// Creating an organization stages it with every field, then verifies and configures
// it, which is when P0 validates the install and reads back the organization's name,
// the key's ID, the allowed domains and the services' status.
func TestOrganizationCreate(t *testing.T) {
	for _, c := range organizationClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &ClickHouseOrganization{}, server)

			resp := create(t, r, rawOf(t, r, c.planned()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake,
				"POST "+basePath+" {}",
				"PUT "+organizationPath+" "+c.body,
				"POST "+organizationPath+"/verify "+c.body,
				"POST "+organizationPath+"/configure "+c.body,
			)
			if want := rawOf(t, r, c.organization(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// When the install check fails, the apply fails with the staged item in state, which
// Terraform taints. The next apply then destroys it before it creates it again, which
// is the only way back: P0 refuses to stage an item that exists.
func TestOrganizationCreateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.failVerify = true
	r := configure(t, &ClickHouseOrganization{}, server)

	resp := create(t, r, rawOf(t, r, awsOrganization.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "ClickHouse Cloud organization install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+organizationPath+" "+awsOrganization.body,
		"POST "+organizationPath+"/verify "+awsOrganization.body,
	)
	if want := rawOf(t, r, awsOrganization.organization(common.StateStage)); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant the staged organization", resp.State.Raw)
	}
}

// P0 refuses to add an organization twice, so one that exists is imported, not
// created, and a refused create saves nothing and leaves P0's item as it was.
func TestOrganizationCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, organizationPath, common.StateInstalled, json.RawMessage(awsOrganization.body))
	r := configure(t, &ClickHouseOrganization{}, server)

	resp := create(t, r, rawOf(t, r, awsOrganization.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage organization component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+basePath+" {}", "PUT "+organizationPath+" "+awsOrganization.body)
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(organizationPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

func TestOrganizationDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, organizationPath, common.StateInstalled, json.RawMessage(awsOrganization.body))
	r := configure(t, &ClickHouseOrganization{}, server)

	resp := destroy(t, r, rawOf(t, r, awsOrganization.organization(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+organizationPath)
	if got := fake.State(organizationPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// An organization that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Its state, and the four
// fields that P0 reads when it validates the install, are unknown in it. Every other
// plan is the framework's, so an installed organization plans no difference.
func TestOrganizationModifyPlan(t *testing.T) {
	r := &ClickHouseOrganization{}
	at := func(state string) tftypes.Value { return rawOf(t, r, awsOrganization.organization(state)) }
	// The plan that finishes the install of an organization at state.
	finishing := rawOf(t, r, awsOrganization.planned())
	none := rawOf(t, r, (*clickhouseOrganizationModel)(nil))

	cases := []struct {
		name  string
		prior tftypes.Value // The refreshed state, null on create.
		plan  tftypes.Value // The framework's plan, null on destroy.
		want  tftypes.Value
	}{
		{name: "installed", prior: at(common.StateInstalled), plan: at(common.StateInstalled), want: at(common.StateInstalled)},
		{name: "configure", prior: at(common.StateConfigure), plan: at(common.StateConfigure), want: finishing},
		{name: "stage", prior: at(common.StateStage), plan: at(common.StateStage), want: finishing},
		{name: "create", prior: none, plan: finishing, want: finishing},
		{name: "destroy", prior: at(common.StateStage), plan: none, want: none},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := modifyPlan(t, r, c.prior, c.plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
			}
			if !resp.Plan.Raw.Equal(c.want) {
				t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, c.want)
			}
		})
	}
}

// An organization that P0 hasn't installed converges in one apply: one imported at
// stage or at configure. Its plan shows an update, the update verifies and configures it without
// staging it, and the plan after that shows no difference.
func TestOrganizationUnfinishedItemConverges(t *testing.T) {
	for _, c := range organizationClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				fake, server := newFakeItems(t)
				fake.seed(t, organizationPath, state, json.RawMessage(c.body))
				r := configure(t, &ClickHouseOrganization{}, server)

				prior := rawOf(t, r, c.organization(state))
				plan := modifyPlan(t, r, prior, prior)
				if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(prior) {
					t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
				}

				resp := update(t, r, prior, plan.Plan.Raw)

				if resp.Diagnostics.HasError() {
					t.Fatalf("Update: %v", resp.Diagnostics)
				}
				wantRequests(t, fake, "POST "+organizationPath+"/verify "+c.body, "POST "+organizationPath+"/configure "+c.body)
				installed := rawOf(t, r, c.organization(common.StateInstalled))
				if !resp.State.Raw.Equal(installed) {
					t.Fatalf("state =\n%v\nwant the installed organization", resp.State.Raw)
				}
				if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
					t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
				}
			})
		}
	}
}

// When the check fails on the update that finishes an install, the apply fails and the
// state keeps the item as it was, so the next plan shows the update again.
func TestOrganizationUpdateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, organizationPath, common.StateStage, json.RawMessage(awsOrganization.body))
	fake.failVerify = true
	r := configure(t, &ClickHouseOrganization{}, server)
	prior := rawOf(t, r, awsOrganization.organization(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsOrganization.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "ClickHouse Cloud organization install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+organizationPath+"/verify "+awsOrganization.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged organization kept", resp.State.Raw)
	}
}
