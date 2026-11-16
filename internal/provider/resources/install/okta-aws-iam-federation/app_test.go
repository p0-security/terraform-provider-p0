package installoktaawsiamfederation

// copy of internal/provider/resources/install/auth0/tenant_test.go, renamed for Okta (AWS IAM
// Federation) (INT-1473), with the Okta app ID's, domain's and client ID's cases.

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

// An app whose private key is in one cloud's secret manager, as Terraform and as
// P0's API hold it.
type appCloud struct {
	name  string
	vault func() *vaultModel
	// The name of the secret that holds the private key.
	secret string
	// The body of every request that carries the app: its connector, its domain, its
	// client ID, its vault and its secret's name.
	body string
}

var awsApp = appCloud{
	name:   "aws",
	vault:  awsVault,
	secret: "okta/acme/p0",
	body: `{"connector":"prod","domain":"acme.okta.com",` +
		`"clientId":"` + testClientId + `",` +
		`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"privateKeyName":"okta/acme/p0"}`,
}

var gcpApp = appCloud{
	name:   "gcp",
	vault:  gcpVault,
	secret: "okta-acme-p0",
	body: `{"connector":"prod","domain":"acme.okta.com",` +
		`"clientId":"` + testClientId + `",` +
		`"vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"privateKeyName":"okta-acme-p0"}`,
}

const testClientId = "0oa9z8y7x6w5v4u3t2s1"

var appClouds = []appCloud{awsApp, gcpApp}

// The app at state, with the entity ID that P0 reads from Okta once it has validated
// the install, and the metadata URL that follows from it.
func (c appCloud) app(state string) *oktaAwsIamFederationAppModel {
	entityId, metadataUrl := types.StringNull(), types.StringNull()
	if state != common.StateStage {
		entityId, metadataUrl = types.StringValue(fakeEntityId), types.StringValue(fakeMetadataUrl)
	}
	return &oktaAwsIamFederationAppModel{
		Id:              types.StringValue(testAppId),
		Connector:       types.StringValue("prod"),
		Domain:          types.StringValue("acme.okta.com"),
		ClientId:        types.StringValue(testClientId),
		Vault:           c.vault(),
		PrivateKeyName:  types.StringValue(c.secret),
		EntityId:        entityId,
		SamlMetadataUrl: metadataUrl,
		State:           types.StringValue(state),
	}
}

// The plan for a new app: everything P0 assigns is unknown.
func (c appCloud) planned() *oktaAwsIamFederationAppModel {
	planned := c.app(common.StateStage)
	planned.EntityId = types.StringUnknown()
	planned.SamlMetadataUrl = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The response of P0's API at state.
func (c appCloud) response(state string) string {
	if state == common.StateStage {
		return `{"item":` + c.body[:len(c.body)-1] + `,"state":"` + state + `"}}`
	}
	return `{"item":` + c.body[:len(c.body)-1] + `,"entityId":"` + fakeEntityId + `","state":"` + state + `"},` +
		`"metadata":{"samlMetadataUrl":"` + fakeMetadataUrl + `"}}`
}

// The PUT that stages the item, and verify and configure, carry the app's
// connector, domain, client ID, vault and secret name, in the field names of the app's install
// schema. `vault` is the shared Vault element, in which "install" names the account or
// the project. None carries what P0 assigns.
func TestAppRequestJson(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&OktaAwsIamFederationApp{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&OktaAwsIamFederationApp{}).toJson(c.app(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed app =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in: the state and, once it has validated the install, the entity ID and the metadata
// URL.
func TestAppFromJson(t *testing.T) {
	for _, c := range appClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item oktaAwsIamFederationAppApi
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&OktaAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item).(*oktaAwsIamFederationAppModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.app(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewOktaAwsIamFederationApp(), got)
			})
		}
	}
}

func TestAppFromJsonRequiresVault(t *testing.T) {
	var diags diag.Diagnostics
	got := (&OktaAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &oktaAwsIamFederationAppApi{Item: &oktaAwsIamFederationAppJson{}})
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
		t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
	}
}

// A vault of a type this provider doesn't know is refused rather than read into the
// wrong fields.
func TestAppFromJsonRequiresAKnownVault(t *testing.T) {
	item := oktaAwsIamFederationAppApi{Item: &oktaAwsIamFederationAppJson{Vault: &vaultJson{Type: "azure-kv", Install: "acme"}}}

	var diags diag.Diagnostics
	got := (&OktaAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item)
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported Okta (AWS IAM Federation) app" {
		t.Errorf("fromJson = %+v, %v; want nil and an unsupported app error", got, diags)
	}
	// The error names the app, its vault and the vaults this provider takes.
	const want = `The Okta (AWS IAM Federation) app 0oa1b2c3d4e5f6g7h8i9 has a "azure-kv" vault. This provider supports only aws-sm and gcp-sm.`
	for _, d := range diags.Errors() {
		if !strings.HasPrefix(d.Detail(), want) {
			t.Errorf("error = %q; want it to start with %q", d.Detail(), want)
		}
	}
}

// P0 fixes the connector, the domain, the client ID, the vault and the secret's name when it
// creates the item, so changing any of them replaces the installation. What P0 assigns
// is read-only.
func TestAppSchema(t *testing.T) {
	s := schemaOf(t, NewOktaAwsIamFederationApp())
	vault := nestedAttributes(t, s, "vault")
	for field, attribute := range map[string]schema.Attribute{
		"id":                   s.Attributes["id"],
		"connector":            s.Attributes["connector"],
		"domain":               s.Attributes["domain"],
		"client_id":            s.Attributes["client_id"],
		"private_key_name":     s.Attributes["private_key_name"],
		"vault.type":           vault["type"],
		"vault.account_id":     vault["account_id"],
		"vault.project_id":     vault["project_id"],
		"vault.secrets_region": vault["secrets_region"],
	} {
		if !requiresReplace(t, attribute) {
			t.Errorf("%s doesn't require replacement", field)
		}
	}

	for _, name := range []string{"entity_id", "saml_metadata_url", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
}

// P0's messages, as its own tests spell them out.
const (
	vaultAccountRule  = "Enter the 12-digit ID of the AWS account the private key is in."
	vaultRegionRule   = "Enter the AWS region the private key is in, such as us-west-2."
	vaultProjectRule  = "Enter the ID of the Google Cloud project the private key is in."
	secretNameRule    = "That doesn't look like a secret name. Enter the name of the secret that holds the private key, not its ARN, its full resource name or the private key itself."
	secretSuffixRule  = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule = "Enter the name of the secret that holds, or will hold, the private key."
	clientIdRule      = "Enter the 20-character client ID of the Okta API service app."
	appIdRule         = "Enter the 20-character ID of the Okta AWS Account Federation app, as its URL in the Okta Admin Console shows it."
	domainRule        = "Enter the org's Okta domain, such as acme.okta.com, in lowercase. A custom domain can't be used."
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
	values := map[string]tftypes.Value{"id": str(testAppId), "connector": str("prod"), "domain": str("acme.okta.com"), "client_id": clientId, "private_key_name": secret}
	return validateConfig(t, &OktaAwsIamFederationApp{}, blocks, values)
}

// P0's own cases for where the private key is
// (packages/integrations/okta-aws-iam-federation/backend/src/install/__tests__/app.test.ts in
// the app), plus the edges of each rule and the fields each vault takes, run through
// ValidateConfig: each refused value gets one error, at its attribute, with P0's
// message. Each check skips a value Terraform doesn't know yet.
func TestAppValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:okta/acme/p0"
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
		{name: "an empty name", vault: awsVault, secret: str(""), want: []string{"private_key_name: " + missingSecretRule}},
		{name: "an empty name with an unknown vault", vault: unknownObject, secret: str(""), want: []string{"private_key_name: " + missingSecretRule}},
		// P0 trims the name whatever the vault, so this doesn't wait for the vault's type.
		{
			name:   "a padded name with an unknown vault type",
			vault:  objectOf(map[string]tftypes.Value{"type": unknownString}),
			secret: str(" okta/secret"),
			want: []string{"private_key_name: P0 removes the whitespace around 'private_key_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an ARN", vault: awsVault, secret: str(arn + "-AbCdEf"), want: []string{"private_key_name: " + secretNameRule}},
		{name: "a name with a space", vault: awsVault, secret: str("okta secret"), want: []string{"private_key_name: " + secretNameRule}},
		{name: "a wildcard", vault: awsVault, secret: str("okta/*"), want: []string{"private_key_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", vault: awsVault, secret: str("okta/secret-AbCdEf"), want: []string{"private_key_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", vault: gcpVault, secret: str("projects/acme-secrets/secrets/okta-secret"), want: []string{"private_key_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", vault: gcpVault, secret: awsSecret, want: []string{"private_key_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", vault: gcpVault, secret: str("okta.secret"), want: []string{"private_key_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", vault: gcpVault, secret: str("okta-secret-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", vault: awsVault, secret: str(" okta/secret-AbCdEf\n"), want: []string{"private_key_name: " + secretSuffixRule}},
		{
			name:   "a name with a trailing newline",
			vault:  awsVault,
			secret: str("okta/secret\n"),
			want: []string{"private_key_name: P0 removes the whitespace around 'private_key_name', so the value it stores " +
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
// pasted the private key in its place.
func TestAppValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret string
		want   []string
	}{
		{name: "an ARN", vault: objectOf(awsVaultFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid secret name for the private key"}},
		{name: "a full resource name", vault: objectOf(gcpVaultFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid secret name for the private key"}},
		{name: "a suffixed name", vault: objectOf(awsVaultFields), secret: "acme/token-Zx9Qw2", want: []string{"Secret name for the private key ends like an ARN suffix"}},
		{name: "a padded name", vault: objectOf(awsVaultFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", vault: objectOf(awsVaultFields), secret: "", want: []string{"Missing secret name for the private key"}},
		{name: "vault account", vault: with(awsVaultFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the private key"}},
		{name: "vault region", vault: with(awsVaultFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the private key"}},
		{name: "vault partition", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the private key"}},
		{name: "vault project", vault: with(gcpVaultFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the private key"}},
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
// (packages/integrations/okta-aws-iam-federation/backend/src/install/__tests__/app.test.ts in the
// app), plus the edges of the rule, run through ValidateConfig. Each refused value gets
// one error, at client_id, with P0's message, which doesn't repeat the value: the
// private key may be pasted in its place.
func TestAppValidateConfigClientId(t *testing.T) {
	const padded = "client_id: P0 removes the whitespace around 'client_id', so the value it stores " +
		"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."

	cases := []struct {
		name     string
		clientId string
		want     []string
	}{
		{name: "a client ID", clientId: testClientId},
		{name: "19 characters", clientId: testClientId[1:], want: []string{"client_id: " + clientIdRule}},
		{name: "21 characters", clientId: testClientId + "a", want: []string{"client_id: " + clientIdRule}},
		{name: "one with a hyphen", clientId: "0oa9z8y7x6w5v4u3t2-1", want: []string{"client_id: " + clientIdRule}},
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

// P0's rules for the app's ID and the org's domain, run through ValidateConfig. Each
// refused value gets one error, with P0's message.
func TestAppValidateConfigIdAndDomain(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		domain string
		want   []string
	}{
		{name: "an app", id: testAppId, domain: "acme.okta.com"},
		{name: "a preview org", id: testAppId, domain: "acme.oktapreview.com"},
		{name: "an EMEA org", id: testAppId, domain: "acme.okta-emea.com"},
		{name: "an app ID of 19 characters", id: testAppId[1:], domain: "acme.okta.com", want: []string{"id: " + appIdRule}},
		{name: "an app ID with a dot", id: "0oa1b2c3d4e5f6g7h8.9", domain: "acme.okta.com", want: []string{"id: " + appIdRule}},
		{name: "a custom domain", id: testAppId, domain: "login.acme.com", want: []string{"domain: " + domainRule}},
		{name: "a domain with capitals", id: testAppId, domain: "Acme.okta.com", want: []string{"domain: " + domainRule}},
		{name: "a URL", id: testAppId, domain: "https://acme.okta.com", want: []string{"domain: " + domainRule}},
		{name: "an admin domain", id: testAppId, domain: "acme-admin.okta.com"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks := map[string]func(tftypes.Object) tftypes.Value{"vault": objectOf(awsVaultFields)}
			values := map[string]tftypes.Value{"id": str(c.id), "connector": str("prod"), "domain": str(c.domain), "client_id": str(testClientId), "private_key_name": str(awsApp.secret)}
			got := errorDetails(validateConfig(t, &OktaAwsIamFederationApp{}, blocks, values))
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
	attributes := nestedAttributes(t, schemaOf(t, NewOktaAwsIamFederationApp()), "vault")
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
// it, which is when P0 validates the install and reads the entity ID.
func TestAppCreate(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &OktaAwsIamFederationApp{}, server)

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
	r := configure(t, &OktaAwsIamFederationApp{}, server)

	resp := create(t, r, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Okta (AWS IAM Federation) app install check failed" {
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
	r := configure(t, &OktaAwsIamFederationApp{}, server)

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
	r := configure(t, &OktaAwsIamFederationApp{}, server)

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
// entity ID and metadata URL that P0 sets when it validates the install, are unknown in
// it. Every other plan is the framework's, so an installed app plans no difference.
func TestAppModifyPlan(t *testing.T) {
	r := &OktaAwsIamFederationApp{}
	at := func(state string) tftypes.Value { return rawOf(t, r, awsApp.app(state)) }
	// The plan that finishes the install of an app at state.
	finishing := rawOf(t, r, awsApp.planned())
	none := rawOf(t, r, (*oktaAwsIamFederationAppModel)(nil))

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
			r := configure(t, &OktaAwsIamFederationApp{}, server)

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
	r := configure(t, &OktaAwsIamFederationApp{}, server)
	prior := rawOf(t, r, awsApp.app(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Okta (AWS IAM Federation) app install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+appPath+"/verify "+awsApp.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged app kept", resp.State.Raw)
	}
}
