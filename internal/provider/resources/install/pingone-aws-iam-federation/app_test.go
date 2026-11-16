package installpingoneawsiamfederation

// copy of internal/provider/resources/install/okta-aws-iam-federation/app_test.go, renamed for
// PingOne (AWS IAM Federation) (INT-1473), with PingOne's IDs, region and role attribute.

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

// An app whose client secret is in one cloud's secret manager, as Terraform and as
// P0's API hold it.
type appCloud struct {
	name  string
	vault func() *vaultModel
	// The name of the secret that holds the client secret.
	secret string
	// The body of every request that carries the app: its connector, its region, its
	// environment, its role attribute, its client ID, its vault and its secret's name.
	body string
}

var awsApp = appCloud{
	name:   "aws",
	vault:  awsVault,
	secret: "pingone/acme/p0",
	body: `{"connector":"prod","region":{"type":"NA"},"environmentId":"` + testEnvironmentId + `","roleAttribute":"awsRoles",` +
		`"clientId":"` + testClientId + `",` +
		`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"clientSecretName":"pingone/acme/p0"}`,
}

var gcpApp = appCloud{
	name:   "gcp",
	vault:  gcpVault,
	secret: "pingone-acme-p0",
	body: `{"connector":"prod","region":{"type":"NA"},"environmentId":"` + testEnvironmentId + `","roleAttribute":"awsRoles",` +
		`"clientId":"` + testClientId + `",` +
		`"vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"clientSecretName":"pingone-acme-p0"}`,
}

const testClientId = "7d6c5b4a-3f2e-4d1c-9b8a-7f6e5d4c3b2a"

var appClouds = []appCloud{awsApp, gcpApp}

// The app at state, with the metadata URL that P0 gives once it has the item.
func (c appCloud) app(state string) *pingOneAwsIamFederationAppModel {
	return &pingOneAwsIamFederationAppModel{
		Id:               types.StringValue(testAppId),
		Connector:        types.StringValue("prod"),
		Region:           types.StringValue("NA"),
		EnvironmentId:    types.StringValue(testEnvironmentId),
		RoleAttribute:    types.StringValue("awsRoles"),
		ClientId:         types.StringValue(testClientId),
		Vault:            c.vault(),
		ClientSecretName: types.StringValue(c.secret),
		SamlMetadataUrl:  types.StringValue(fakeMetadataUrl),
		State:            types.StringValue(state),
	}
}

// The plan for a new app: everything P0 assigns is unknown.
func (c appCloud) planned() *pingOneAwsIamFederationAppModel {
	planned := c.app(common.StateStage)
	planned.SamlMetadataUrl = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The response of P0's API at state.
func (c appCloud) response(state string) string {
	return `{"item":` + c.body[:len(c.body)-1] + `,"state":"` + state + `"},` +
		`"metadata":{"samlMetadataUrl":"` + fakeMetadataUrl + `"}}`
}

// The PUT that stages the item, and verify and configure, carry the app's
// connector, region, environment, role attribute, client ID, vault and secret name, in the field names of the app's install
// schema. `vault` is the shared Vault element, in which "install" names the account or
// the project. None carries what P0 assigns.
func TestAppRequestJson(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&PingOneAwsIamFederationApp{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&PingOneAwsIamFederationApp{}).toJson(c.app(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed app =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in: the state and the metadata URL.
func TestAppFromJson(t *testing.T) {
	for _, c := range appClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item pingOneAwsIamFederationAppApi
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&PingOneAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item).(*pingOneAwsIamFederationAppModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.app(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewPingOneAwsIamFederationApp(), got)
			})
		}
	}
}

func TestAppFromJsonRequiresVault(t *testing.T) {
	var diags diag.Diagnostics
	got := (&PingOneAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &pingOneAwsIamFederationAppApi{Item: &pingOneAwsIamFederationAppJson{}})
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
		t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
	}
}

// A vault of a type this provider doesn't know is refused rather than read into the
// wrong fields.
func TestAppFromJsonRequiresAKnownVault(t *testing.T) {
	item := pingOneAwsIamFederationAppApi{Item: &pingOneAwsIamFederationAppJson{Vault: &vaultJson{Type: "azure-kv", Install: "acme"}}}

	var diags diag.Diagnostics
	got := (&PingOneAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item)
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported PingOne (AWS IAM Federation) app" {
		t.Errorf("fromJson = %+v, %v; want nil and an unsupported app error", got, diags)
	}
	// The error names the app, its vault and the vaults this provider takes.
	want := "The PingOne (AWS IAM Federation) app " + testAppId + ` has a "azure-kv" vault. This provider supports only aws-sm and gcp-sm.`
	for _, d := range diags.Errors() {
		if !strings.HasPrefix(d.Detail(), want) {
			t.Errorf("error = %q; want it to start with %q", d.Detail(), want)
		}
	}
}

// P0 fixes the connector, the region, the environment, the role attribute, the client
// ID, the vault and the secret's name when it
// creates the item, so changing any of them replaces the installation. What P0 assigns
// is read-only.
func TestAppSchema(t *testing.T) {
	s := schemaOf(t, NewPingOneAwsIamFederationApp())
	vault := nestedAttributes(t, s, "vault")
	for field, attribute := range map[string]schema.Attribute{
		"id":                   s.Attributes["id"],
		"connector":            s.Attributes["connector"],
		"region":               s.Attributes["region"],
		"environment_id":       s.Attributes["environment_id"],
		"role_attribute":       s.Attributes["role_attribute"],
		"client_id":            s.Attributes["client_id"],
		"client_secret_name":   s.Attributes["client_secret_name"],
		"vault.type":           vault["type"],
		"vault.account_id":     vault["account_id"],
		"vault.project_id":     vault["project_id"],
		"vault.secrets_region": vault["secrets_region"],
	} {
		if !requiresReplace(t, attribute) {
			t.Errorf("%s doesn't require replacement", field)
		}
	}

	for _, name := range []string{"saml_metadata_url", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
}

// P0's messages, as its own tests spell them out.
const (
	vaultAccountRule  = "Enter the 12-digit ID of the AWS account the client secret is in."
	vaultRegionRule   = "Enter the AWS region the client secret is in, such as us-west-2."
	vaultProjectRule  = "Enter the ID of the Google Cloud project the client secret is in."
	secretNameRule    = "That doesn't look like a secret name. Enter the name of the secret that holds the client secret, not its ARN, its full resource name or the client secret itself."
	secretSuffixRule  = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule = "Enter the name of the secret that holds, or will hold, the client secret."
	clientIdRule      = "Enter the client ID of the PingOne worker app."
	appIdRule         = "Enter the ID of the PingOne SAML app that signs users in to AWS, as its Configuration tab in the PingOne admin console shows it."
	environmentRule   = "Enter the ID of the PingOne environment that holds the app, as its Environment Properties in the PingOne admin console show it."
	roleAttributeRule = "Enter the name of a PingOne user attribute: a letter, then up to 99 letters, digits and underscores."
)

var (
	awsVaultFields = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpVaultFields = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}
)

// Runs the app's ValidateConfig against a configuration with the given vault
// and secret name.
func validateApp(t *testing.T, vault func(tftypes.Object) tftypes.Value, secret tftypes.Value) diag.Diagnostics {
	t.Helper()
	return validateAppWithClient(t, vault, str(testClientId), secret)
}

// Runs the app's ValidateConfig against a configuration with the given vault, client
// ID and secret name.
func validateAppWithClient(t *testing.T, vault func(tftypes.Object) tftypes.Value, clientId tftypes.Value, secret tftypes.Value) diag.Diagnostics {
	t.Helper()
	blocks := map[string]func(tftypes.Object) tftypes.Value{"vault": vault}
	values := map[string]tftypes.Value{"id": str(testAppId), "connector": str("prod"), "region": str("NA"), "environment_id": str(testEnvironmentId), "role_attribute": str("awsRoles"), "client_id": clientId, "client_secret_name": secret}
	return validateConfig(t, &PingOneAwsIamFederationApp{}, blocks, values)
}

// P0's own cases for where the client secret is
// (packages/integrations/pingone-aws-iam-federation/backend/src/install/__tests__/app.test.ts in
// the app), plus the edges of each rule and the fields each vault takes, run through
// ValidateConfig: each refused value gets one error, at its attribute, with P0's
// message. Each check skips a value Terraform doesn't know yet.
func TestAppValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:pingone/acme/p0"
	awsVault, gcpVault := objectOf(awsVaultFields), objectOf(gcpVaultFields)
	awsSecret, gcpSecret := str(awsApp.secret), str(gcpApp.secret)

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
		{name: "an empty name", vault: awsVault, secret: str(""), want: []string{"client_secret_name: " + missingSecretRule}},
		{name: "an empty name with an unknown vault", vault: unknownObject, secret: str(""), want: []string{"client_secret_name: " + missingSecretRule}},
		// P0 trims the name whatever the vault, so this doesn't wait for the vault's type.
		{
			name:   "a padded name with an unknown vault type",
			vault:  objectOf(map[string]tftypes.Value{"type": unknownString}),
			secret: str(" pingone/secret"),
			want: []string{"client_secret_name: P0 removes the whitespace around 'client_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an ARN", vault: awsVault, secret: str(arn + "-AbCdEf"), want: []string{"client_secret_name: " + secretNameRule}},
		{name: "a name with a space", vault: awsVault, secret: str("pingone secret"), want: []string{"client_secret_name: " + secretNameRule}},
		{name: "a wildcard", vault: awsVault, secret: str("pingone/*"), want: []string{"client_secret_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", vault: awsVault, secret: str("pingone/secret-AbCdEf"), want: []string{"client_secret_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", vault: gcpVault, secret: str("projects/acme-secrets/secrets/pingone-secret"), want: []string{"client_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", vault: gcpVault, secret: awsSecret, want: []string{"client_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", vault: gcpVault, secret: str("pingone.secret"), want: []string{"client_secret_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", vault: gcpVault, secret: str("pingone-secret-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", vault: awsVault, secret: str(" pingone/secret-AbCdEf\n"), want: []string{"client_secret_name: " + secretSuffixRule}},
		{
			name:   "a name with a trailing newline",
			vault:  awsVault,
			secret: str("pingone/secret\n"),
			want: []string{"client_secret_name: P0 removes the whitespace around 'client_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateApp(t, c.vault, c.secret))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Terraform shows a nested attribute's error at its resource, so each summary says what
// the value is for. No error repeats the secret's name, because a customer may have
// pasted the client secret in its place.
func TestAppValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret string
		want   []string
	}{
		{name: "an ARN", vault: objectOf(awsVaultFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid secret name for the client secret"}},
		{name: "a full resource name", vault: objectOf(gcpVaultFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid secret name for the client secret"}},
		{name: "a suffixed name", vault: objectOf(awsVaultFields), secret: "acme/token-Zx9Qw2", want: []string{"Secret name for the client secret ends like an ARN suffix"}},
		{name: "a padded name", vault: objectOf(awsVaultFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", vault: objectOf(awsVaultFields), secret: "", want: []string{"Missing secret name for the client secret"}},
		{name: "vault account", vault: with(awsVaultFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the client secret"}},
		{name: "vault region", vault: with(awsVaultFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the client secret"}},
		{name: "vault partition", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the client secret"}},
		{name: "vault project", vault: with(gcpVaultFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the client secret"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateApp(t, c.vault, str(c.secret))
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

// P0's own cases for the client ID
// (packages/integrations/pingone-aws-iam-federation/backend/src/install/__tests__/app.test.ts in the
// app), plus the edges of the rule, run through ValidateConfig. Each refused value gets
// one error, at client_id, with P0's message, which doesn't repeat the value: the
// client secret may be pasted in its place.
func TestAppValidateConfigClientId(t *testing.T) {
	const padded = "client_id: P0 removes the whitespace around 'client_id', so the value it stores " +
		"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."

	cases := []struct {
		name     string
		clientId string
		want     []string
	}{
		{name: "a client ID", clientId: testClientId},
		{name: "a client ID in capitals", clientId: strings.ToUpper(testClientId)},
		{name: "a client ID without hyphens", clientId: strings.ReplaceAll(testClientId, "-", ""), want: []string{"client_id: " + clientIdRule}},
		{name: "a client ID that's too short", clientId: testClientId[1:], want: []string{"client_id: " + clientIdRule}},
		{name: "empty", clientId: "", want: []string{"client_id: " + clientIdRule}},
		// P0 trims the client ID before it checks it, so a padded one is refused for its
		// whitespace, and one that's only whitespace for its form.
		{name: "a padded client ID", clientId: " " + testClientId + "\n", want: []string{padded}},
		{name: "only whitespace", clientId: " ", want: []string{"client_id: " + clientIdRule}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateAppWithClient(t, objectOf(awsVaultFields), str(c.clientId), str(awsApp.secret))
			if got := errorDetails(diags); !reflect.DeepEqual(got, append([]string{}, c.want...)) {
				t.Errorf("errors =\n%v\nwant\n%v", got, c.want)
			}
			for _, d := range diags.Errors() {
				if value := strings.TrimSpace(c.clientId); value != "" && strings.Contains(d.Summary()+d.Detail(), value) {
					t.Errorf("error %v repeats the value", d)
				}
			}
		})
	}
}

// P0's rules for the app's ID, the environment's ID and the role attribute, run through
// ValidateConfig. Each refused value gets one error, with P0's message.
func TestAppValidateConfigIdsAndRoleAttribute(t *testing.T) {
	cases := []struct {
		name          string
		id            string
		environmentId string
		roleAttribute string
		want          []string
	}{
		{name: "an app", id: testAppId, environmentId: testEnvironmentId, roleAttribute: "awsRoles"},
		{name: "a role attribute with digits and underscores", id: testAppId, environmentId: testEnvironmentId, roleAttribute: "aws_roles_2"},
		{name: "an app ID that isn't a UUID", id: "app-1", environmentId: testEnvironmentId, roleAttribute: "awsRoles", want: []string{"id: " + appIdRule}},
		{name: "an environment ID that isn't a UUID", id: testAppId, environmentId: "acme", roleAttribute: "awsRoles", want: []string{"environment_id: " + environmentRule}},
		{name: "a role attribute that starts with a digit", id: testAppId, environmentId: testEnvironmentId, roleAttribute: "2roles", want: []string{"role_attribute: " + roleAttributeRule}},
		{name: "a role attribute with a hyphen", id: testAppId, environmentId: testEnvironmentId, roleAttribute: "aws-roles", want: []string{"role_attribute: " + roleAttributeRule}},
		{name: "a role attribute that's too long", id: testAppId, environmentId: testEnvironmentId, roleAttribute: "a" + strings.Repeat("b", 100), want: []string{"role_attribute: " + roleAttributeRule}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks := map[string]func(tftypes.Object) tftypes.Value{"vault": objectOf(awsVaultFields)}
			values := map[string]tftypes.Value{"id": str(c.id), "connector": str("prod"), "region": str("NA"), "environment_id": str(c.environmentId), "role_attribute": str(c.roleAttribute), "client_id": str(testClientId), "client_secret_name": str(awsApp.secret)}
			got := errorDetails(validateConfig(t, &PingOneAwsIamFederationApp{}, blocks, values))
			if !reflect.DeepEqual(got, append([]string{}, c.want...)) {
				t.Errorf("errors =\n%v\nwant\n%v", got, c.want)
			}
		})
	}
}

// A client ID that Terraform doesn't know yet is checked when it is.
func TestAppValidateConfigSkipsAnUnknownClientId(t *testing.T) {
	if diags := validateAppWithClient(t, objectOf(awsVaultFields), unknownString, str(awsApp.secret)); diags.HasError() {
		t.Errorf("errors = %v; want none", diags)
	}
}

// Every field a rule names is one the vault has. A misspelt name would read a null
// value, which skips the check silently.
func TestVaultRuleFieldNames(t *testing.T) {
	names := awsApp.vault().fields()
	attributes := nestedAttributes(t, schemaOf(t, NewPingOneAwsIamFederationApp()), "vault")
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

// Creating an app stages it with every field, then verifies and configures
// it, which is when P0 validates the install.
func TestAppCreate(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &PingOneAwsIamFederationApp{}, server)

			resp := create(t, r, rawOf(t, r, c.planned()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake,
				"POST "+basePath+" {}",
				"PUT "+appPath+" "+c.body,
				"POST "+appPath+"/verify "+c.body,
				"POST "+appPath+"/configure "+c.body,
			)
			if want := rawOf(t, r, c.app(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// When the install check fails, the apply fails with the staged item in state, which
// Terraform taints. The next apply then destroys it before it creates it again, which
// is the only way back: P0 refuses to stage an item that exists.
func TestAppCreateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.failVerify = true
	r := configure(t, &PingOneAwsIamFederationApp{}, server)

	resp := create(t, r, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "PingOne (AWS IAM Federation) app install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+appPath+" "+awsApp.body,
		"POST "+appPath+"/verify "+awsApp.body,
	)
	if want := rawOf(t, r, awsApp.app(common.StateStage)); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant the staged app", resp.State.Raw)
	}
}

// P0 refuses to add an app twice, so one that exists is imported, not
// created, and a refused create saves nothing and leaves P0's item as it was.
func TestAppCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, appPath, common.StateInstalled, json.RawMessage(awsApp.body))
	r := configure(t, &PingOneAwsIamFederationApp{}, server)

	resp := create(t, r, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage app component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+basePath+" {}", "PUT "+appPath+" "+awsApp.body)
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(appPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

func TestAppDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, appPath, common.StateInstalled, json.RawMessage(awsApp.body))
	r := configure(t, &PingOneAwsIamFederationApp{}, server)

	resp := destroy(t, r, rawOf(t, r, awsApp.app(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+appPath)
	if got := fake.State(appPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// An app that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Its state, and the
// metadata URL, which P0 gives from the start, stays known. Every other plan is the
// framework's, so an installed app plans no difference.
func TestAppModifyPlan(t *testing.T) {
	r := &PingOneAwsIamFederationApp{}
	at := func(state string) tftypes.Value { return rawOf(t, r, awsApp.app(state)) }
	// The plan that finishes the install of an app at state.
	unfinished := awsApp.app(common.StateStage)
	unfinished.State = types.StringUnknown()
	finishing := rawOf(t, r, unfinished)
	none := rawOf(t, r, (*pingOneAwsIamFederationAppModel)(nil))

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

// An app imported at stage converges in one apply: its plan shows an update,
// the update verifies and configures it without staging it, and the plan after that
// shows no difference.
func TestAppStagedItemConverges(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, appPath, common.StateStage, json.RawMessage(c.body))
			r := configure(t, &PingOneAwsIamFederationApp{}, server)

			staged := rawOf(t, r, c.app(common.StateStage))
			plan := modifyPlan(t, r, staged, staged)
			if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
				t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
			}

			resp := update(t, r, staged, plan.Plan.Raw)

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+appPath+"/verify "+c.body, "POST "+appPath+"/configure "+c.body)
			installed := rawOf(t, r, c.app(common.StateInstalled))
			if !resp.State.Raw.Equal(installed) {
				t.Fatalf("state =\n%v\nwant the installed app", resp.State.Raw)
			}
			if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
				t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
			}
		})
	}
}

// When the check fails on the update that finishes an install, the apply fails and the
// state keeps the item as it was, so the next plan shows the update again.
func TestAppUpdateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, appPath, common.StateStage, json.RawMessage(awsApp.body))
	fake.failVerify = true
	r := configure(t, &PingOneAwsIamFederationApp{}, server)
	prior := rawOf(t, r, awsApp.app(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "PingOne (AWS IAM Federation) app install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+appPath+"/verify "+awsApp.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged app kept", resp.State.Raw)
	}
}
