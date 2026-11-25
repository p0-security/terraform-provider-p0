package installpingfederateawsiamfederation

// copy of internal/provider/resources/install/pingfederate-aws-iam-federation/app_test.go, renamed for
// PingFederate (AWS IAM Federation) (INT-1565), with PingFederate's URL, role attribute,
// SCIM user name and CA certificate in place of PingOne's IDs, region and client ID.

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

// An app whose SCIM password is in one cloud's secret manager, as Terraform and as
// P0's API hold it.
type appCloud struct {
	name  string
	vault func() *vaultModel
	// The name of the secret that holds the SCIM password.
	secret string
	// The body of every request that carries the app: PingFederate's URL, and its SCIM
	// role source with the connector, role attribute, SCIM user name, vault, secret's
	// name and CA certificate.
	body string
	// The app's CA certificate, which the AWS app has and the Google Cloud app doesn't.
	caCertificate types.String
}

const testCaCertificate = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"

var awsApp = appCloud{
	name:   "aws",
	vault:  awsVault,
	secret: "pingfederate/acme/p0",
	body: `{"pingFederateUrl":"` + testPingFederateUrl + `","roleSource":{"type":"scim","connector":"prod","roleAttribute":"awsRole",` +
		`"scimUsername":"p0",` +
		`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"scimPasswordName":"pingfederate/acme/p0",` +
		`"caCertificate":"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"}}`,
	caCertificate: types.StringValue(testCaCertificate),
}

var gcpApp = appCloud{
	name:   "gcp",
	vault:  gcpVault,
	secret: "pingfederate-acme-p0",
	body: `{"pingFederateUrl":"` + testPingFederateUrl + `","roleSource":{"type":"scim","connector":"prod","roleAttribute":"awsRole",` +
		`"scimUsername":"p0",` +
		`"vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"scimPasswordName":"pingfederate-acme-p0"}}`,
	caCertificate: types.StringNull(),
}

var appClouds = []appCloud{awsApp, gcpApp}

// The app at state, with the metadata URL that P0 gives once it has the item.
func (c appCloud) app(state string) *pingFederateAwsIamFederationAppModel {
	return &pingFederateAwsIamFederationAppModel{
		Id:              types.StringValue(testAppId),
		PingFederateUrl: types.StringValue(testPingFederateUrl),
		Scim: &scimModel{
			Connector:        types.StringValue("prod"),
			RoleAttribute:    types.StringValue("awsRole"),
			ScimUsername:     types.StringValue("p0"),
			Vault:            c.vault(),
			ScimPasswordName: types.StringValue(c.secret),
			CaCertificate:    c.caCertificate,
		},
		SamlMetadataUrl: types.StringValue(fakeMetadataUrl),
		State:           types.StringValue(state),
	}
}

// The plan for a new app: everything P0 assigns is unknown.
func (c appCloud) planned() *pingFederateAwsIamFederationAppModel {
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

// The PUT that stages the item, and verify and configure, carry PingFederate's URL
// and the SCIM role source's connector, role attribute, SCIM user name, vault, secret
// name and CA certificate, in the field names of the app's install schema. `vault` is the shared Vault element, in which "install" names the account or
// the project. None carries what P0 assigns.
func TestAppRequestJson(t *testing.T) {
	for _, c := range appClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&PingFederateAwsIamFederationApp{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&PingFederateAwsIamFederationApp{}).toJson(c.app(common.StateInstalled))); got != c.body {
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
				var item pingFederateAwsIamFederationAppApi
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&PingFederateAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item).(*pingFederateAwsIamFederationAppModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.app(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewPingFederateAwsIamFederationApp(), got)
			})
		}
	}
}

func TestAppFromJsonRequiresVault(t *testing.T) {
	var diags diag.Diagnostics
	got := (&PingFederateAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &pingFederateAwsIamFederationAppApi{Item: &pingFederateAwsIamFederationAppJson{RoleSource: &roleSourceJson{Type: scimRoleSourceType}}})
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
		t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
	}
}

// A vault of a type this provider doesn't know is refused rather than read into the
// wrong fields.
func TestAppFromJsonRequiresAKnownVault(t *testing.T) {
	item := pingFederateAwsIamFederationAppApi{Item: &pingFederateAwsIamFederationAppJson{RoleSource: &roleSourceJson{Type: scimRoleSourceType, Vault: &vaultJson{Type: "azure-kv", Install: "acme"}}}}

	var diags diag.Diagnostics
	got := (&PingFederateAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item)
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported PingFederate (AWS IAM Federation) app" {
		t.Errorf("fromJson = %+v, %v; want nil and an unsupported app error", got, diags)
	}
	// The error names the app, its vault and the vaults this provider takes.
	want := "The PingFederate (AWS IAM Federation) app " + testAppId + ` has a "azure-kv" vault. This provider supports only aws-sm and gcp-sm.`
	for _, d := range diags.Errors() {
		if !strings.HasPrefix(d.Detail(), want) {
			t.Errorf("error = %q; want it to start with %q", d.Detail(), want)
		}
	}
}

// P0 fixes the app's ID, PingFederate's URL and every SCIM field when it creates the
// item, so changing any of them replaces the installation. An attribute source's fields
// change in place. What P0 assigns is read-only.
func TestAppSchema(t *testing.T) {
	s := schemaOf(t, NewPingFederateAwsIamFederationApp())
	scim := nestedAttributes(t, s, "scim")
	vault := as[schema.SingleNestedAttribute](t, scim["vault"]).Attributes
	attributeSource := nestedAttributes(t, s, "attribute_source")
	for field, attribute := range map[string]schema.Attribute{
		"id":                        s.Attributes["id"],
		"pingfederate_url":          s.Attributes["pingfederate_url"],
		"scim.connector":            scim["connector"],
		"scim.role_attribute":       scim["role_attribute"],
		"scim.scim_username":        scim["scim_username"],
		"scim.scim_password_name":   scim["scim_password_name"],
		"scim.ca_certificate":       scim["ca_certificate"],
		"scim.vault.type":           vault["type"],
		"scim.vault.account_id":     vault["account_id"],
		"scim.vault.project_id":     vault["project_id"],
		"scim.vault.secrets_region": vault["secrets_region"],
	} {
		if !requiresReplace(t, attribute) {
			t.Errorf("%s doesn't require replacement", field)
		}
	}
	for _, name := range []string{"entity_id", "issuer", "client_id", "jwks"} {
		if requiresReplace(t, attributeSource[name]) {
			t.Errorf("attribute_source.%s requires replacement", name)
		}
	}

	for name, attribute := range map[string]schema.Attribute{
		"saml_metadata_url":          s.Attributes["saml_metadata_url"],
		"state":                      s.Attributes["state"],
		"attribute_source.audience":  attributeSource["audience"],
		"attribute_source.roles_url": attributeSource["roles_url"],
	} {
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
}

// P0's messages, as its own tests spell them out.
const (
	vaultAccountRule  = "Enter the 12-digit ID of the AWS account the SCIM password is in."
	vaultRegionRule   = "Enter the AWS region the SCIM password is in, such as us-west-2."
	vaultProjectRule  = "Enter the ID of the Google Cloud project the SCIM password is in."
	secretNameRule    = "That doesn't look like a secret name. Enter the name of the secret that holds the SCIM password, not its ARN, its full resource name or the password itself."
	secretSuffixRule  = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule = "Enter the name of the secret that holds, or will hold, the SCIM password."
	urlRule           = "Enter PingFederate's runtime base URL, such as https://sso.example.com:9031."
	roleAttributeRule = "Enter the attribute's name: a letter, then up to 99 letters, digits and hyphens. Active Directory has an attribute of the same name."
	scimUsernameRule  = "Enter the user name of the SCIM connection's HTTP Basic credentials, with no spaces or colons."
	caCertificateRule = "Enter one or more PEM certificates, each starting with -----BEGIN CERTIFICATE-----, or leave it empty."
)

var (
	awsVaultFields = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpVaultFields = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}
)

// The app's own fields, as a configuration sets them.
func appValues() map[string]tftypes.Value {
	return map[string]tftypes.Value{"id": str(testAppId), "pingfederate_url": str(testPingFederateUrl)}
}

// The scim block's fields other than the vault and the secret's name.
func scimValues() map[string]tftypes.Value {
	return map[string]tftypes.Value{"connector": str("prod"), "role_attribute": str("awsRole"), "scim_username": str("p0")}
}

// The scim block, with vault and fields.
func scimBlock(vault func(tftypes.Object) tftypes.Value, fields map[string]tftypes.Value) func(tftypes.Object) tftypes.Value {
	return func(typ tftypes.Object) tftypes.Value {
		vaultType, ok := typ.AttributeTypes["vault"].(tftypes.Object)
		if !ok {
			panic("the scim block has no vault object")
		}
		values := map[string]tftypes.Value{"vault": vault(vaultType)}
		for name, value := range fields {
			values[name] = value
		}
		return objectOf(values)(typ)
	}
}

// Runs the app's ValidateConfig against a SCIM app with the given vault and secret
// name.
func validateApp(t *testing.T, vault func(tftypes.Object) tftypes.Value, secret tftypes.Value) diag.Diagnostics {
	t.Helper()
	fields := scimValues()
	fields["scim_password_name"] = secret
	blocks := map[string]func(tftypes.Object) tftypes.Value{"scim": scimBlock(vault, fields)}
	return validateConfig(t, &PingFederateAwsIamFederationApp{}, blocks, appValues())
}

// P0's own cases for where the SCIM password is
// (packages/integrations/pingfederate-aws-iam-federation/backend/src/install/__tests__/app.test.ts in
// the app), plus the edges of each rule and the fields each vault takes, run through
// ValidateConfig: each refused value gets one error, at its attribute, with P0's
// message. Each check skips a value Terraform doesn't know yet.
func TestAppValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:pingfederate/acme/p0"
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
		{name: "an account that isn't an AWS account ID", vault: with(awsVaultFields, "account_id", str("acme-secrets")), secret: awsSecret, want: []string{"scim.vault.account_id: " + vaultAccountRule}},
		{name: "a Google Cloud region on AWS", vault: with(awsVaultFields, "secrets_region", str("us-west1")), secret: awsSecret, want: []string{"scim.vault.secrets_region: " + vaultRegionRule}},
		{name: "a GovCloud region", vault: with(awsVaultFields, "secrets_region", str("us-gov-west-1")), secret: awsSecret, want: []string{"scim.vault.secrets_region: " + unsupported}},
		{name: "a project that isn't a project ID", vault: with(gcpVaultFields, "project_id", str("123456789012")), secret: gcpSecret, want: []string{"scim.vault.project_id: " + vaultProjectRule}},

		// The fields each vault takes.
		{name: "aws without an account", vault: without(awsVaultFields, "account_id"), secret: awsSecret, want: []string{`scim.vault.account_id: 'scim.vault.account_id' is required when 'scim.vault.type' is "aws-sm".`}},
		{name: "aws without a region", vault: without(awsVaultFields, "secrets_region"), secret: awsSecret, want: []string{`scim.vault.secrets_region: 'scim.vault.secrets_region' is required when 'scim.vault.type' is "aws-sm".`}},
		{
			name:   "aws with a project",
			vault:  with(awsVaultFields, "project_id", str("acme-secrets")),
			secret: awsSecret,
			want:   []string{`scim.vault.project_id: 'scim.vault.project_id' may only be set when 'scim.vault.type' is "gcp-sm".`},
		},
		{name: "gcp without a project", vault: without(gcpVaultFields, "project_id"), secret: gcpSecret, want: []string{`scim.vault.project_id: 'scim.vault.project_id' is required when 'scim.vault.type' is "gcp-sm".`}},
		{
			name:   "gcp with an account",
			vault:  with(gcpVaultFields, "account_id", str("123456789012")),
			secret: gcpSecret,
			want:   []string{`scim.vault.account_id: 'scim.vault.account_id' may only be set when 'scim.vault.type' is "aws-sm".`},
		},
		{
			name:   "gcp with a region",
			vault:  with(gcpVaultFields, "secrets_region", str("us-west-2")),
			secret: gcpSecret,
			want:   []string{`scim.vault.secrets_region: 'scim.vault.secrets_region' may only be set when 'scim.vault.type' is "aws-sm".`},
		},

		// The secret's name, which each secret manager has its own rule for.
		{name: "an empty name", vault: awsVault, secret: str(""), want: []string{"scim.scim_password_name: " + missingSecretRule}},
		{name: "an empty name with an unknown vault", vault: unknownObject, secret: str(""), want: []string{"scim.scim_password_name: " + missingSecretRule}},
		// P0 trims the name whatever the vault, so this doesn't wait for the vault's type.
		{
			name:   "a padded name with an unknown vault type",
			vault:  objectOf(map[string]tftypes.Value{"type": unknownString}),
			secret: str(" pingfederate/secret"),
			want: []string{"scim.scim_password_name: P0 removes the whitespace around 'scim.scim_password_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an ARN", vault: awsVault, secret: str(arn + "-AbCdEf"), want: []string{"scim.scim_password_name: " + secretNameRule}},
		{name: "a name with a space", vault: awsVault, secret: str("pingfederate secret"), want: []string{"scim.scim_password_name: " + secretNameRule}},
		{name: "a wildcard", vault: awsVault, secret: str("pingfederate/*"), want: []string{"scim.scim_password_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", vault: awsVault, secret: str("pingfederate/secret-AbCdEf"), want: []string{"scim.scim_password_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", vault: gcpVault, secret: str("projects/acme-secrets/secrets/pingfederate-secret"), want: []string{"scim.scim_password_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", vault: gcpVault, secret: awsSecret, want: []string{"scim.scim_password_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", vault: gcpVault, secret: str("pingfederate.secret"), want: []string{"scim.scim_password_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", vault: gcpVault, secret: str("pingfederate-secret-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", vault: awsVault, secret: str(" pingfederate/secret-AbCdEf\n"), want: []string{"scim.scim_password_name: " + secretSuffixRule}},
		{
			name:   "a name with a trailing newline",
			vault:  awsVault,
			secret: str("pingfederate/secret\n"),
			want: []string{"scim.scim_password_name: P0 removes the whitespace around 'scim.scim_password_name', so the value it stores " +
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
// pasted the SCIM password in its place.
func TestAppValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret string
		want   []string
	}{
		{name: "an ARN", vault: objectOf(awsVaultFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid secret name for the SCIM password"}},
		{name: "a full resource name", vault: objectOf(gcpVaultFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid secret name for the SCIM password"}},
		{name: "a suffixed name", vault: objectOf(awsVaultFields), secret: "acme/token-Zx9Qw2", want: []string{"Secret name for the SCIM password ends like an ARN suffix"}},
		{name: "a padded name", vault: objectOf(awsVaultFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", vault: objectOf(awsVaultFields), secret: "", want: []string{"Missing secret name for the SCIM password"}},
		{name: "vault account", vault: with(awsVaultFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the SCIM password"}},
		{name: "vault region", vault: with(awsVaultFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the SCIM password"}},
		{name: "vault partition", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the SCIM password"}},
		{name: "vault project", vault: with(gcpVaultFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the SCIM password"}},
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

// P0's own cases for PingFederate's URL, the role attribute, the SCIM user name and the
// CA certificate
// (packages/integrations/pingfederate-aws-iam-federation/backend/src/install/__tests__/app.test.ts in the
// app), plus the edges of each rule, run through ValidateConfig. Each refused value gets
// one error, at its attribute, with P0's message, which doesn't repeat the value: the
// SCIM password may be pasted in its place.
func TestAppValidateConfigFields(t *testing.T) {
	const padded = "scim.role_attribute: P0 removes the whitespace around 'scim.role_attribute', so the value it stores " +
		"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."

	cases := []struct {
		name  string
		field string
		value tftypes.Value
		want  []string
	}{
		{name: "an app", field: "role_attribute", value: str("awsRole")},
		{name: "a URL with a path", field: "pingfederate_url", value: str("https://sso.example.com/pf")},
		{name: "an http URL", field: "pingfederate_url", value: str("http://sso.example.com"), want: []string{"pingfederate_url: " + urlRule}},
		{name: "a URL with a query", field: "pingfederate_url", value: str("https://sso.example.com?a=b"), want: []string{"pingfederate_url: " + urlRule}},
		{name: "a role attribute with digits and hyphens", field: "role_attribute", value: str("aws-role-2")},
		{name: "a role attribute with a space", field: "role_attribute", value: str("aws role"), want: []string{"scim.role_attribute: " + roleAttributeRule}},
		{name: "a role attribute with an underscore", field: "role_attribute", value: str("aws_role"), want: []string{"scim.role_attribute: " + roleAttributeRule}},
		{name: "a role attribute that starts with a digit", field: "role_attribute", value: str("2roles"), want: []string{"scim.role_attribute: " + roleAttributeRule}},
		{name: "a role attribute that's too long", field: "role_attribute", value: str("a" + strings.Repeat("b", 100)), want: []string{"scim.role_attribute: " + roleAttributeRule}},
		{name: "a padded role attribute", field: "role_attribute", value: str(" awsRole\n"), want: []string{padded}},
		{name: "a SCIM user name with a colon", field: "scim_username", value: str("p0:admin"), want: []string{"scim.scim_username: " + scimUsernameRule}},
		{name: "a SCIM user name with a space", field: "scim_username", value: str("p0 admin"), want: []string{"scim.scim_username: " + scimUsernameRule}},
		{name: "a CA certificate", field: "ca_certificate", value: str(testCaCertificate)},
		{name: "an empty CA certificate", field: "ca_certificate", value: str("")},
		{name: "a CA certificate that isn't PEM", field: "ca_certificate", value: str("MIIB"), want: []string{"scim.ca_certificate: " + caCertificateRule}},
		{
			name:  "a CA certificate with a trailing newline",
			field: "ca_certificate",
			value: str(testCaCertificate + "\n"),
			want: []string{"scim.ca_certificate: P0 removes the whitespace around 'scim.ca_certificate', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an unknown URL", field: "pingfederate_url", value: unknownString},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			values, fields := appValues(), scimValues()
			fields["scim_password_name"] = str(awsApp.secret)
			if _, ok := values[c.field]; ok {
				values[c.field] = c.value
			} else {
				fields[c.field] = c.value
			}
			blocks := map[string]func(tftypes.Object) tftypes.Value{"scim": scimBlock(objectOf(awsVaultFields), fields)}
			diags := validateConfig(t, &PingFederateAwsIamFederationApp{}, blocks, values)
			if got := errorDetails(diags); !reflect.DeepEqual(got, append([]string{}, c.want...)) {
				t.Errorf("errors =\n%v\nwant\n%v", got, c.want)
			}
		})
	}
}

// Every field a rule names is one the vault has. A misspelt name would read a null
// value, which skips the check silently.
func TestVaultRuleFieldNames(t *testing.T) {
	names := awsApp.vault().fields()
	vaultAttribute := nestedAttributes(t, schemaOf(t, NewPingFederateAwsIamFederationApp()), "scim")["vault"]
	attributes := as[schema.SingleNestedAttribute](t, vaultAttribute).Attributes
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
			r := configure(t, &PingFederateAwsIamFederationApp{}, server)

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
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)

	resp := create(t, r, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "PingFederate (AWS IAM Federation) app install check failed" {
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
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)

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
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)

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
	r := &PingFederateAwsIamFederationApp{}
	at := func(state string) tftypes.Value { return rawOf(t, r, awsApp.app(state)) }
	// The plan that finishes the install of an app at state.
	unfinished := awsApp.app(common.StateStage)
	unfinished.State = types.StringUnknown()
	finishing := rawOf(t, r, unfinished)
	none := rawOf(t, r, (*pingFederateAwsIamFederationAppModel)(nil))

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
			r := configure(t, &PingFederateAwsIamFederationApp{}, server)

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
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)
	prior := rawOf(t, r, awsApp.app(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsApp.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "PingFederate (AWS IAM Federation) app install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+appPath+"/verify "+awsApp.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged app kept", resp.State.Raw)
	}
}
