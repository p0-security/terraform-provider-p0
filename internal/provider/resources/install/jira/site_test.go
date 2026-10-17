package installjira

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

func awsModel() *siteModel {
	return &siteModel{
		Site: types.StringValue("acme.atlassian.net"),
		Hosting: &installapp.ConnectorHostingWithServiceAccountModel{
			ConnectorHostingModel: installapp.ConnectorHostingModel{
				Type:                types.StringValue(installapp.AwsHosting),
				AccountId:           types.StringValue("123456789012"),
				ProjectId:           types.StringNull(),
				ConnectorName:       types.StringValue("p0-jira-acme"),
				ConnectorRegion:     types.StringValue("us-west-2"),
				ConnectorServiceUri: types.StringNull(),
			},
			ConnectorServiceAccount: types.StringNull(),
		},
		Vault: &vaultModel{
			Type:          types.StringValue(awsSecretsManager),
			AccountId:     types.StringValue("123456789012"),
			SecretsRegion: types.StringValue("us-west-2"),
			ProjectId:     types.StringNull(),
		},
		ClientSecretName: types.StringValue("p0/jira/acme"),
		DomainPattern:    types.StringValue(`example\.com`),
		ClientId:         types.StringValue("AbCdEfGhIjKlMnOpQrStUvWxYz012345"),
		CloudId:          types.StringNull(),
		State:            types.StringNull(),
	}
}

func gcpModel() *siteModel {
	return &siteModel{
		Site: types.StringValue("acme.atlassian.net"),
		Hosting: &installapp.ConnectorHostingWithServiceAccountModel{
			ConnectorHostingModel: installapp.ConnectorHostingModel{
				Type:                types.StringValue(installapp.GcpHosting),
				AccountId:           types.StringNull(),
				ProjectId:           types.StringValue("acme-prod"),
				ConnectorName:       types.StringValue("p0-jira-acme"),
				ConnectorRegion:     types.StringValue("us-west1"),
				ConnectorServiceUri: types.StringNull(),
			},
			ConnectorServiceAccount: types.StringNull(),
		},
		Vault: &vaultModel{
			Type:          types.StringValue("gcp-sm"),
			AccountId:     types.StringNull(),
			SecretsRegion: types.StringNull(),
			ProjectId:     types.StringValue("acme-secrets"),
		},
		ClientSecretName: types.StringValue("jira-acme"),
		DomainPattern:    types.StringValue(`example\.com`),
		ClientId:         types.StringValue("AbCdEfGhIjKlMnOpQrStUvWxYz012345"),
		CloudId:          types.StringNull(),
		State:            types.StringNull(),
	}
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(got)
}

const (
	awsHostingJson = `"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-jira-acme","connectorRegion":"us-west-2"}`
	awsVaultJson   = `"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"}`
	gcpHostingJson = `"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-acme","connectorRegion":"us-west1"}`
	gcpVaultJson   = `"vault":{"type":"gcp-sm","install":"acme-secrets"}`
)

// The PUT that stages the item, and verify, carry the whole item but the client ID, in
// the field names of the app's install schema, and nothing P0 sets.
func TestSiteItemJson(t *testing.T) {
	cases := map[string]struct {
		model *siteModel
		want  string
	}{
		"aws": {
			model: awsModel(),
			want:  `{` + awsHostingJson + `,` + awsVaultJson + `,"clientSecretName":"p0/jira/acme","domainPattern":"example\\.com"}`,
		},
		"gcp": {
			model: gcpModel(),
			want:  `{` + gcpHostingJson + `,` + gcpVaultJson + `,"clientSecretName":"jira-acme","domainPattern":"example\\.com"}`,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			c.model.CloudId = types.StringValue("cloud")
			c.model.State = types.StringValue(common.StateInstalled)
			c.model.Hosting.ConnectorServiceUri = types.StringValue("https://p0-jira-acme.a.run.app")
			if got := marshal(t, itemJson(c.model)); got != c.want {
				t.Errorf("PUT and verify body =\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

// Configure carries the whole item with the client ID, and nothing P0 sets.
func TestSiteConfigureJson(t *testing.T) {
	model := gcpModel()
	model.CloudId = types.StringValue("cloud")
	model.Hosting.ConnectorServiceAccount = types.StringValue("p0-jira-acme@acme-prod.iam.gserviceaccount.com")
	want := `{` + gcpHostingJson + `,` + gcpVaultJson + `,"clientSecretName":"jira-acme","domainPattern":"example\\.com","clientId":"AbCdEfGhIjKlMnOpQrStUvWxYz012345"}`
	if got := marshal(t, siteToJson(model)); got != want {
		t.Errorf("configure body =\n%s\nwant\n%s", got, want)
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in.
func TestSiteFromJson(t *testing.T) {
	gcpInstalled := gcpModel()
	gcpInstalled.Hosting.ConnectorServiceAccount = types.StringValue("p0-jira-acme@acme-prod.iam.gserviceaccount.com")
	gcpInstalled.Hosting.ConnectorServiceUri = types.StringValue("https://p0-jira-acme.a.run.app")
	gcpInstalled.CloudId = types.StringValue("cloud")
	gcpInstalled.State = types.StringValue(common.StateInstalled)
	awsStaged := awsModel()
	awsStaged.ClientId = types.StringNull()
	awsStaged.State = types.StringValue(common.StateStage)

	cases := map[string]struct {
		response string
		want     *siteModel
	}{
		"gcp installed": {
			response: `{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-acme","connectorRegion":"us-west1",` +
				`"connectorServiceAccount":"p0-jira-acme@acme-prod.iam.gserviceaccount.com","connectorServiceUri":"https://p0-jira-acme.a.run.app"},` +
				gcpVaultJson + `,"clientSecretName":"jira-acme","domainPattern":"example\\.com",` +
				`"clientId":"AbCdEfGhIjKlMnOpQrStUvWxYz012345","cloudId":"cloud","label":"acme.atlassian.net","state":"installed"}`,
			want: gcpInstalled,
		},
		// A site added in the P0 app has no client ID until its configure step.
		"aws staged": {
			response: `{` + awsHostingJson + `,` + awsVaultJson + `,"clientSecretName":"p0/jira/acme","domainPattern":"example\\.com","state":"stage"}`,
			want:     awsStaged,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var item siteJson
			if err := json.Unmarshal([]byte(c.response), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			var diags diag.Diagnostics
			got, ok := siteFromJson(context.Background(), &diags, "acme.atlassian.net", &item).(*siteModel)
			if !ok || diags.HasError() {
				t.Fatalf("fromJson failed: %v", diags)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("fromJson =\n%+v\nwant\n%+v", got, c.want)
			}
		})
	}
}

// An item without a vault or hosting, or with one that this provider doesn't support,
// such as one changed in the P0 app, is refused rather than read into the wrong
// fields.
func TestSiteFromJsonRefusals(t *testing.T) {
	cases := map[string]struct {
		response string
		summary  string
	}{
		"no vault":      {response: `{` + awsHostingJson + `}`, summary: "Bad API response"},
		"no hosting":    {response: `{` + awsVaultJson + `}`, summary: "Bad API response"},
		"azure vault":   {response: `{` + awsHostingJson + `,"vault":{"type":"azure-kv","install":"vault"}}`, summary: "Unsupported Jira site"},
		"azure hosting": {response: `{"hosting":{"type":"azure","connectorName":"p0-jira","connectorRegion":"eastus"},` + awsVaultJson + `}`, summary: "Unsupported Jira site"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var item siteJson
			if err := json.Unmarshal([]byte(c.response), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			var diags diag.Diagnostics
			got := siteFromJson(context.Background(), &diags, "acme.atlassian.net", &item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != c.summary {
				t.Errorf("fromJson = %+v, %v; want nil and %q", got, diags, c.summary)
			}
		})
	}
}

func siteSchema(t *testing.T) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	NewSite().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestSchemaIsValid(t *testing.T) {
	if diags := siteSchema(t).ValidateImplementation(context.Background()); diags.HasError() {
		t.Errorf("ValidateImplementation: %v", diags)
	}
}

// P0 fixes the site, the hosting, the vault, the secret and the domain pattern when it
// creates the item, so changing any of them replaces the installation. The client ID
// changes in place.
func TestSchemaReplacement(t *testing.T) {
	ctx := context.Background()
	attributes := siteSchema(t).Attributes
	requiresReplace := stringplanmodifier.RequiresReplace().Description(ctx)

	nested := func(name string) map[string]schema.Attribute {
		attribute, ok := attributes[name].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is not a nested attribute", name)
		}
		return attribute.Attributes
	}
	replaces := func(attribute schema.Attribute) bool {
		str, ok := attribute.(schema.StringAttribute)
		if !ok {
			t.Fatalf("%v is not a string attribute", attribute)
		}
		for _, modifier := range str.PlanModifiers {
			if modifier.Description(ctx) == requiresReplace {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name      string
		attribute schema.Attribute
		want      bool
	}{
		{name: "site", attribute: attributes["site"], want: true},
		{name: "client_secret_name", attribute: attributes["client_secret_name"], want: true},
		{name: "domain_pattern", attribute: attributes["domain_pattern"], want: true},
		{name: "client_id", attribute: attributes["client_id"], want: false},
		{name: "cloud_id", attribute: attributes["cloud_id"], want: false},
		{name: "hosting.type", attribute: nested("hosting")["type"], want: true},
		{name: "hosting.account_id", attribute: nested("hosting")["account_id"], want: true},
		{name: "hosting.project_id", attribute: nested("hosting")["project_id"], want: true},
		{name: "hosting.connector_name", attribute: nested("hosting")["connector_name"], want: true},
		{name: "hosting.connector_region", attribute: nested("hosting")["connector_region"], want: true},
		{name: "vault.type", attribute: nested("vault")["type"], want: true},
		{name: "vault.account_id", attribute: nested("vault")["account_id"], want: true},
		{name: "vault.secrets_region", attribute: nested("vault")["secrets_region"], want: true},
		{name: "vault.project_id", attribute: nested("vault")["project_id"], want: true},
	}

	for _, c := range cases {
		if got := replaces(c.attribute); got != c.want {
			t.Errorf("%s requires replacement = %v; want %v", c.name, got, c.want)
		}
	}
}

// A nested object with the given fields set and every other field null.
func objectOf(fields map[string]tftypes.Value) func(tftypes.Object) tftypes.Value {
	return func(typ tftypes.Object) tftypes.Value {
		values := map[string]tftypes.Value{}
		for name, attrType := range typ.AttributeTypes {
			values[name] = tftypes.NewValue(attrType, nil)
		}
		for name, value := range fields {
			values[name] = value
		}
		return tftypes.NewValue(typ, values)
	}
}

// A nested object that Terraform only knows at apply time, such as one set from a
// module output.
func unknownObject(typ tftypes.Object) tftypes.Value {
	return tftypes.NewValue(typ, tftypes.UnknownValue)
}

func str(value string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, value)
}

var unknownString = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

var (
	awsHostingFields = map[string]tftypes.Value{"type": str("aws"), "account_id": str("123456789012"), "connector_name": str("p0-jira-acme"), "connector_region": str("us-west-2")}
	gcpHostingFields = map[string]tftypes.Value{"type": str("gcp"), "project_id": str("acme-prod"), "connector_name": str("p0-jira-acme"), "connector_region": str("us-west1")}
	awsVaultFields   = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpVaultFields   = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}

	awsHosting = objectOf(awsHostingFields)
	gcpHosting = objectOf(gcpHostingFields)
	awsVault   = objectOf(awsVaultFields)
	gcpVault   = objectOf(gcpVaultFields)
)

// A nested object with base's fields, one of them replaced by value.
func with(base map[string]tftypes.Value, field string, value tftypes.Value) func(tftypes.Object) tftypes.Value {
	fields := map[string]tftypes.Value{field: value}
	for name, baseValue := range base {
		if name != field {
			fields[name] = baseValue
		}
	}
	return objectOf(fields)
}

// Runs the resource's ValidateConfig against a configuration with the given hosting
// and vault. overrides replaces the root string attributes, which are otherwise valid
// for AWS. The attribute validators don't run here, because Terraform runs those
// separately.
func validateConfig(t *testing.T, hosting, vault func(tftypes.Object) tftypes.Value, overrides map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	siteSchema := siteSchema(t)

	objectType, ok := siteSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	nestedType := func(name string) tftypes.Object {
		typ, ok := objectType.AttributeTypes[name].(tftypes.Object)
		if !ok {
			t.Fatalf("%s is not an object", name)
		}
		return typ
	}

	fields := map[string]tftypes.Value{
		"site":               str("acme.atlassian.net"),
		"client_secret_name": str("p0/jira/acme"),
		"domain_pattern":     str(`example\.com`),
		"client_id":          str("AbCdEfGhIjKlMnOpQrStUvWxYz012345"),
		"hosting":            hosting(nestedType("hosting")),
		"vault":              vault(nestedType("vault")),
	}
	for name, value := range overrides {
		fields[name] = value
	}

	resp := &resource.ValidateConfigResponse{}
	config := tfsdk.Config{Schema: siteSchema, Raw: objectOf(fields)(objectType)}
	(&Site{}).ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	return resp.Diagnostics
}

// Terraform calls ValidateConfig while planning, before the provider calls P0, so these
// are the plan-time checks. Each skips a value Terraform doesn't know yet.
func TestSiteValidateConfig(t *testing.T) {
	unknownType := objectOf(map[string]tftypes.Value{"type": unknownString})
	secret := func(value tftypes.Value) map[string]tftypes.Value {
		return map[string]tftypes.Value{"client_secret_name": value}
	}
	otherAccount := with(awsVaultFields, "account_id", str("210987654321"))

	cases := []struct {
		name      string
		hosting   func(tftypes.Object) tftypes.Value
		vault     func(tftypes.Object) tftypes.Value
		overrides map[string]tftypes.Value
		// The summaries of the errors expected, in any order.
		want []string
	}{
		{name: "aws", hosting: awsHosting, vault: awsVault},
		{name: "gcp", hosting: gcpHosting, vault: gcpVault, overrides: secret(str("jira-acme"))},
		{name: "unknown hosting", hosting: unknownObject, vault: awsVault},
		{name: "unknown vault", hosting: awsHosting, vault: unknownObject},
		{name: "unknown hosting type", hosting: unknownType, vault: gcpVault, overrides: secret(str("jira-acme"))},
		{name: "unknown vault type", hosting: gcpHosting, vault: unknownType},

		// The vault's cloud must be the connector's.
		{name: "gcp vault, aws hosting", hosting: awsHosting, vault: gcpVault, overrides: secret(str("jira-acme")), want: []string{"Vault in another cloud than the connector"}},
		{name: "aws vault, gcp hosting", hosting: gcpHosting, vault: awsVault, want: []string{"Vault in another cloud than the connector"}},
		{name: "gcp vault, unknown hosting", hosting: unknownObject, vault: gcpVault, overrides: secret(str("jira-acme"))},
		{name: "aws vault, hosting of unknown type", hosting: unknownType, vault: awsVault},

		{name: "unknown site", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"site": unknownString}},
		{name: "site with a path", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"site": str("acme.atlassian.net/jira")}, want: []string{invalidSiteHostSummary}},
		// P0 checks the item's ID as it is.
		{name: "site with a trailing space", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"site": str("acme.atlassian.net ")}, want: []string{invalidSiteHostSummary}},

		{name: "aws hosting without an account", hosting: with(awsHostingFields, "account_id", tftypes.NewValue(tftypes.String, nil)), vault: awsVault, want: []string{"Missing AWS account ID"}},
		{name: "aws account that isn't one", hosting: with(awsHostingFields, "account_id", str("12345")), vault: awsVault, want: []string{"Invalid AWS account ID for the connector"}},
		{name: "aws connector region that isn't one", hosting: with(awsHostingFields, "connector_region", str("us-west1")), vault: awsVault, want: []string{"Invalid AWS region for the connector"}},
		{name: "govcloud connector", hosting: with(awsHostingFields, "connector_region", str("us-gov-west-1")), vault: awsVault, want: []string{"Unsupported AWS region for the connector"}},
		{name: "aws connector name with an underscore", hosting: with(awsHostingFields, "connector_name", str("p0_jira")), vault: awsVault, want: []string{"Invalid connector name"}},
		{name: "unknown connector name", hosting: with(awsHostingFields, "connector_name", unknownString), vault: awsVault},
		{name: "connector name with whitespace", hosting: with(awsHostingFields, "connector_name", str("p0-jira-acme ")), vault: awsVault, want: []string{"Whitespace around a value"}},
		{name: "gcp project with an underscore", hosting: with(gcpHostingFields, "project_id", str("acme_prod")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid Google Cloud project for the connector"}},
		{name: "gcp project in capitals", hosting: with(gcpHostingFields, "project_id", str("Acme-Prod")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid Google Cloud project for the connector"}},
		{name: "gcp project number", hosting: with(gcpHostingFields, "project_id", str("123456789012")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid Google Cloud project for the connector"}},
		{name: "blank gcp project", hosting: with(gcpHostingFields, "project_id", str("")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid Google Cloud project for the connector"}},
		{name: "gcp zone", hosting: with(gcpHostingFields, "connector_region", str("us-west1-a")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid Google Cloud region for the connector"}},
		{name: "gcp connector name too long for a service account", hosting: with(gcpHostingFields, "connector_name", str("p0-jira-connector-for-acme-production")), vault: gcpVault, overrides: secret(str("jira")), want: []string{"Invalid connector name"}},

		{name: "aws vault without a region", hosting: awsHosting, vault: with(awsVaultFields, "secrets_region", tftypes.NewValue(tftypes.String, nil)), want: []string{"Missing vault field"}},
		{name: "aws vault with a project", hosting: awsHosting, vault: with(awsVaultFields, "project_id", str("acme-secrets")), want: []string{"Unexpected vault field"}},
		{name: "gcp vault without a project", hosting: gcpHosting, vault: objectOf(map[string]tftypes.Value{"type": str("gcp-sm")}), overrides: secret(str("jira")), want: []string{"Missing vault field"}},
		{name: "china secret", hosting: awsHosting, vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), want: []string{"Unsupported AWS region for the client secret"}},
		{name: "secret's account that isn't one", hosting: awsHosting, vault: with(awsVaultFields, "account_id", str("acme")), want: []string{"Invalid AWS account ID for the client secret"}},
		{name: "secret's project in capitals", hosting: gcpHosting, vault: with(gcpVaultFields, "project_id", str("Acme-Secrets")), overrides: secret(str("jira")), want: []string{"Invalid Google Cloud project for the client secret"}},
		{name: "unknown secrets region", hosting: awsHosting, vault: with(awsVaultFields, "secrets_region", unknownString)},

		{name: "secret arn", hosting: awsHosting, vault: awsVault, overrides: secret(str("arn:aws:secretsmanager:us-west-2:123456789012:secret:jira")), want: []string{invalidSecretNameSummary}},
		{name: "pasted client secret", hosting: awsHosting, vault: awsVault, overrides: secret(str("ATOAabc def")), want: []string{invalidSecretNameSummary}},
		{name: "gcp secret with a slash", hosting: gcpHosting, vault: gcpVault, overrides: secret(str("jira/acme")), want: []string{invalidSecretNameSummary}},
		{name: "secret name with an unknown vault", hosting: awsHosting, vault: unknownObject, overrides: secret(str("p0/jira/acme"))},
		{name: "unknown secret name", hosting: awsHosting, vault: awsVault, overrides: secret(unknownString)},
		// The connector reads a secret in its own account by its name, and one in another
		// by its partial ARN.
		{name: "secret named like an ARN suffix", hosting: awsHosting, vault: awsVault, overrides: secret(str("p0/jira/acme-AbCdEf"))},
		{name: "secret in another account named like an ARN suffix", hosting: awsHosting, vault: otherAccount, overrides: secret(str("p0/jira/acme-AbCdEf")), want: []string{"Client secret name ends like an ARN suffix"}},
		{name: "secret in another account", hosting: awsHosting, vault: otherAccount},
		{name: "secret named like an ARN suffix, unknown connector account", hosting: with(awsHostingFields, "account_id", unknownString), vault: otherAccount, overrides: secret(str("p0/jira/acme-AbCdEf"))},
		{name: "secret name with a trailing newline", hosting: awsHosting, vault: awsVault, overrides: secret(str("jira/acme\n")), want: []string{"Whitespace around a value"}},

		{name: "blank domain pattern", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"domain_pattern": str(" ")}, want: []string{"Missing domain pattern"}},
		{name: "long domain pattern", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"domain_pattern": str(strings.Repeat("a", 1001))}, want: []string{"Domain pattern too long"}},
		// JavaScript counts this as 1000 UTF-16 code units, and Go as 2000 bytes.
		{name: "domain pattern of 1000 code units", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"domain_pattern": str(strings.Repeat("é", 1000))}},
		{name: "unknown domain pattern", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"domain_pattern": unknownString}},

		{name: "client id with a space", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"client_id": str("abc def")}, want: []string{"Invalid client ID"}},
		{name: "client id over 256 characters", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"client_id": str(strings.Repeat("a", 257))}, want: []string{"Invalid client ID"}},
		{name: "client id with a trailing newline", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"client_id": str("abc\n")}, want: []string{"Whitespace around a value"}},
		{name: "blank client id", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"client_id": str("")}, want: []string{"Missing client ID"}},
		{name: "unknown client id", hosting: awsHosting, vault: awsVault, overrides: map[string]tftypes.Value{"client_id": unknownString}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := []string{}
			for _, d := range validateConfig(t, c.hosting, c.vault, c.overrides).Errors() {
				got = append(got, d.Summary())
			}
			want := append([]string{}, c.want...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors = %v; want %v", got, want)
			}
		})
	}
}

// A refused value gets P0's message, at its attribute, and isn't repeated back: a
// secret's name or a client ID may be the client secret.
func TestSiteValidateConfigWording(t *testing.T) {
	const pasted = "ATOAx9Zq0Jk7LmNpQrStUvWx"
	cases := []struct {
		name      string
		hosting   func(tftypes.Object) tftypes.Value
		vault     func(tftypes.Object) tftypes.Value
		overrides map[string]tftypes.Value
		at        path.Path
		detail    string
	}{
		{
			name: "pasted secret", hosting: awsHosting, vault: awsVault,
			overrides: map[string]tftypes.Value{"client_secret_name": str(pasted + " =")},
			at:        path.Root("client_secret_name"), detail: invalidSecretName,
		},
		{
			name: "pasted secret in google", hosting: gcpHosting, vault: gcpVault,
			overrides: map[string]tftypes.Value{"client_secret_name": str(pasted + "/")},
			at:        path.Root("client_secret_name"), detail: invalidGcpSecretName,
		},
		{
			name: "pasted secret as client id", hosting: awsHosting, vault: awsVault,
			overrides: map[string]tftypes.Value{"client_id": str(pasted + " x")},
			at:        path.Root("client_id"), detail: invalidClientId,
		},
		{
			name: "vault in another cloud", hosting: gcpHosting, vault: awsVault,
			at: path.Root("vault").AtName("type"), detail: vaultForCloud[installapp.GcpHosting],
		},
		{
			name: "site", hosting: awsHosting, vault: awsVault,
			overrides: map[string]tftypes.Value{"site": str("https://acme.atlassian.net")},
			at:        path.Root("site"), detail: invalidSiteHost,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := validateConfig(t, c.hosting, c.vault, c.overrides).Errors()
			if len(errs) != 1 {
				t.Fatalf("errors = %v; want one", errs)
			}
			if errs[0].Detail() != c.detail {
				t.Errorf("error = %q; want %q", errs[0].Detail(), c.detail)
			}
			withPath, ok := errs[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(c.at) {
				t.Errorf("error %v isn't at %s", errs[0], c.at)
			}
			if strings.Contains(errs[0].Summary()+errs[0].Detail(), pasted) {
				t.Errorf("error %v repeats the value", errs[0])
			}
		})
	}
}

// The wording names the site and shows P0's message, without presuming that the
// connector is at fault.
func TestDescribeCheckError(t *testing.T) {
	const message = "400 Bad Request: The Jira site acme.atlassian.net failed its install check while reading the OAuth client secret: The connector can't read the secret."
	summary, detail := describeCheckError("acme.atlassian.net", errors.New(message))
	if summary != "Jira install check failed" {
		t.Errorf("summary = %q", summary)
	}
	if want := "P0 rejected the install check for the Jira site \"acme.atlassian.net\":\n\n" + message; detail != want {
		t.Errorf("detail =\n%s\nwant\n%s", detail, want)
	}
}

// model at state.
func at(model *siteModel, state types.String) *siteModel {
	model.State = state
	return model
}

// model as the resource's state or plan holds it, or null for a nil model.
func rawOf(t *testing.T, model *siteModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	siteSchema := siteSchema(t)
	state := tfsdk.State{Schema: siteSchema, Raw: tftypes.NewValue(siteSchema.Type().TerraformType(ctx), nil)}
	if model == nil {
		return state.Raw
	}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("Set: %v", diags)
	}
	return state.Raw
}

// Runs ModifyPlan on the framework's plan for prior as the framework does, which starts
// the response's plan as its own.
func modifyPlan(t *testing.T, prior, planned tftypes.Value) *resource.ModifyPlanResponse {
	t.Helper()
	siteSchema := siteSchema(t)
	req := resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: siteSchema, Raw: planned},
		State: tfsdk.State{Schema: siteSchema, Raw: prior},
	}
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: siteSchema, Raw: planned.Copy()}}
	(&Site{}).ModifyPlan(context.Background(), req, resp)
	return resp
}

// An item that P0 hasn't installed plans an update even when its configuration hasn't
// changed, so that the next apply finishes the install. What the update's verify and
// configure set, the state, the cloud ID and the connector's URL, is unknown in it.
// Every other plan is the framework's, so an installed item plans no difference.
func TestSiteModifyPlan(t *testing.T) {
	installed := types.StringValue(common.StateInstalled)
	configured := types.StringValue(common.StateConfigure)
	staged := types.StringValue(common.StateStage)
	unknown := types.StringUnknown()

	gcpAt := func(state types.String) *siteModel {
		model := at(gcpModel(), state)
		model.Hosting.ConnectorServiceAccount = types.StringValue("p0-jira-acme@acme-prod.iam.gserviceaccount.com")
		return model
	}
	gcpInstalled := func() *siteModel {
		model := gcpAt(installed)
		model.CloudId = types.StringValue("cloud")
		model.Hosting.ConnectorServiceUri = types.StringValue("https://p0-jira-acme.a.run.app")
		return model
	}
	// What an unfinished item's plan leaves to P0.
	finishing := func(model *siteModel) *siteModel {
		model.State = unknown
		model.CloudId = unknown
		model.Hosting.ConnectorServiceUri = unknown
		return model
	}

	cases := []struct {
		name  string
		prior *siteModel // The refreshed state, nil on create.
		plan  *siteModel // The framework's plan, nil on destroy.
		want  *siteModel
	}{
		{name: "installed", prior: gcpInstalled(), plan: gcpInstalled(), want: gcpInstalled()},
		{name: "configure", prior: gcpAt(configured), plan: gcpAt(configured), want: finishing(gcpAt(configured))},
		{name: "stage", prior: gcpAt(staged), plan: gcpAt(staged), want: finishing(gcpAt(staged))},
		{name: "stage on aws", prior: at(awsModel(), staged), plan: at(awsModel(), staged), want: finishing(at(awsModel(), staged))},
		{name: "create", prior: nil, plan: finishing(gcpAt(staged)), want: finishing(gcpAt(staged))},
		{name: "destroy", prior: gcpAt(staged), plan: nil, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := modifyPlan(t, rawOf(t, c.prior), rawOf(t, c.plan))
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
			}
			if want := rawOf(t, c.want); !resp.Plan.Raw.Equal(want) {
				t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, want)
			}
		})
	}
}

// A hosting block that Terraform only knows at apply time stays unknown.
func TestSiteModifyPlanUnknownHosting(t *testing.T) {
	ctx := context.Background()
	siteSchema := siteSchema(t)
	hostingType, ok := siteSchema.Attributes["hosting"].GetType().(types.ObjectType)
	if !ok {
		t.Fatalf("hosting is not an object")
	}
	withUnknownHosting := func(model *siteModel) tftypes.Value {
		plan := tfsdk.Plan{Schema: siteSchema, Raw: rawOf(t, model)}
		if diags := plan.SetAttribute(ctx, path.Root("hosting"), types.ObjectUnknown(hostingType.AttrTypes)); diags.HasError() {
			t.Fatalf("SetAttribute: %v", diags)
		}
		return plan.Raw
	}

	prior := rawOf(t, at(awsModel(), types.StringValue(common.StateConfigure)))
	resp := modifyPlan(t, prior, withUnknownHosting(at(awsModel(), types.StringValue(common.StateConfigure))))

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
	}
	want := at(awsModel(), types.StringUnknown())
	want.CloudId = types.StringUnknown()
	if !resp.Plan.Raw.Equal(withUnknownHosting(want)) {
		t.Errorf("plan =\n%v\nwant the hosting unknown, and the state and cloud ID with it", resp.Plan.Raw)
	}
}
