package installtailscale

// copy of internal/provider/resources/install/auth0/tenant_test.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686): the credential's types in place of the tenant's
// fields, and a refused change in place of a replacement.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A tailnet with one kind of credential, as Terraform and as P0's API hold it.
type tailnetCase struct {
	name       string
	credential func() *credentialModel
	// The body of every request that carries the tailnet: its connector and its
	// credential.
	body string
}

const testClientId = "kAbc1CNTRL"

var federatedTailnet = tailnetCase{
	name: "federated",
	credential: func() *credentialModel {
		return &credentialModel{
			Type:             types.StringValue(federatedCredential),
			ClientId:         types.StringValue(testClientId),
			ClientSecretName: types.StringNull(),
		}
	},
	body: `{"connector":"prod","credential":{"type":"federated","clientId":"` + testClientId + `"}}`,
}

var awsOAuthTailnet = tailnetCase{
	name: "aws oauth client",
	credential: func() *credentialModel {
		return &credentialModel{
			Type:             types.StringValue(oauthClientCredential),
			ClientId:         types.StringValue(testClientId),
			Vault:            awsVault(),
			ClientSecretName: types.StringValue("tailscale/acme/oauth"),
		}
	},
	body: `{"connector":"prod",` +
		`"credential":{"type":"oauth-client","clientId":"` + testClientId + `",` +
		`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"clientSecretName":"tailscale/acme/oauth"}}`,
}

var gcpOAuthTailnet = tailnetCase{
	name: "gcp oauth client",
	credential: func() *credentialModel {
		return &credentialModel{
			Type:             types.StringValue(oauthClientCredential),
			ClientId:         types.StringValue(testClientId),
			Vault:            gcpVault(),
			ClientSecretName: types.StringValue("tailscale-acme-oauth"),
		}
	},
	body: `{"connector":"prod",` +
		`"credential":{"type":"oauth-client","clientId":"` + testClientId + `",` +
		`"vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"clientSecretName":"tailscale-acme-oauth"}}`,
}

var tailnetCases = []tailnetCase{federatedTailnet, awsOAuthTailnet, gcpOAuthTailnet}

// The tailnet at state, with what P0's discovery finds once it has validated the
// install. Until then, P0 stores every generated field empty.
func (c tailnetCase) tailnet(state string) *tailscaleTailnetModel {
	discovery, dnsName := types.StringValue(""), types.StringValue("")
	if state != common.StateStage {
		discovery, dnsName = types.StringValue(fakeDiscovery), types.StringValue(fakeDnsName)
	}
	return &tailscaleTailnetModel{
		Id:         types.StringValue(testTailnet),
		Connector:  types.StringValue("prod"),
		Credential: c.credential(),
		Discovery:  discovery,
		DnsName:    dnsName,
		State:      types.StringValue(state),
	}
}

// The plan for a new tailnet: everything P0 assigns is unknown.
func (c tailnetCase) planned() *tailscaleTailnetModel {
	planned := c.tailnet(common.StateStage)
	planned.Discovery = types.StringUnknown()
	planned.DnsName = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The item as P0's API answers at state.
func (c tailnetCase) response(state string) string {
	discovery, dnsName := "", ""
	if state != common.StateStage {
		discovery, dnsName = fakeDiscovery, fakeDnsName
	}
	return c.body[:len(c.body)-1] + `,"discovery":"` + discovery + `","dnsName":"` + dnsName + `","state":"` + state + `"}`
}

// The PUT that stages the item, and verify and configure, carry the tailnet's
// connector and credential, in the field names of the app's install schema: the
// credential is a select element, whose option is its "type", and
// `vault` is the shared Vault element, in which "install" names the account or the
// project. None carries what P0 assigns.
func TestTailnetRequestJson(t *testing.T) {
	for _, c := range tailnetCases {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&TailscaleTailnet{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&TailscaleTailnet{}).toJson(c.tailnet(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed tailnet =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in: the state and, once it has validated the install, what discovery found.
func TestTailnetFromJson(t *testing.T) {
	for _, c := range tailnetCases {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item tailscaleTailnetJson
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&TailscaleTailnet{}).fromJson(context.Background(), &diags, testTailnet, &item).(*tailscaleTailnetModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.tailnet(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewTailscaleTailnet(), got)
			})
		}
	}
}

func TestTailnetFromJsonRequiresCredential(t *testing.T) {
	cases := map[string]tailscaleTailnetJson{
		"no credential":                   {},
		"an OAuth client without a vault": {Credential: &credentialJson{Type: oauthClientCredential}},
	}

	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := (&TailscaleTailnet{}).fromJson(context.Background(), &diags, testTailnet, &item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
				t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
			}
		})
	}
}

// A credential or a vault of a kind this provider doesn't know is refused rather than
// read into the wrong fields. The error names the tailnet, what it has and what this
// provider takes.
func TestTailnetFromJsonRequiresKnownKinds(t *testing.T) {
	cases := map[string]struct {
		item tailscaleTailnetJson
		want string
	}{
		"credential": {
			item: tailscaleTailnetJson{Credential: &credentialJson{Type: "api-key"}},
			want: `The Tailscale tailnet T1234CNTRL has a "api-key" credential. This provider supports only federated and oauth-client.`,
		},
		"vault": {
			item: tailscaleTailnetJson{Credential: &credentialJson{Type: oauthClientCredential, Vault: &vaultJson{Type: "azure-kv", Install: "acme"}}},
			want: `The Tailscale tailnet T1234CNTRL has a "azure-kv" vault. This provider supports only aws-sm and gcp-sm.`,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := (&TailscaleTailnet{}).fromJson(context.Background(), &diags, testTailnet, &c.item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported Tailscale tailnet" {
				t.Fatalf("fromJson = %+v, %v; want nil and an unsupported tailnet error", got, diags)
			}
			if detail := diags.Errors()[0].Detail(); !strings.HasPrefix(detail, c.want) {
				t.Errorf("error = %q; want it to start with %q", detail, c.want)
			}
		})
	}
}

// A new ID is another tailnet, so it replaces this one. P0 fixes the connector and the
// credential when it adds the tailnet, and ModifyPlan refuses a change to them, so
// neither plans a replacement. What P0 assigns is read-only.
func TestTailnetSchema(t *testing.T) {
	s := schemaOf(t, NewTailscaleTailnet())
	if !requiresReplace(t, s.Attributes["id"]) {
		t.Errorf("id doesn't require replacement")
	}

	credential := nestedAttributes(t, s, "credential")
	vault, ok := credential["vault"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("credential.vault is not a nested attribute")
	}
	fixed := map[string]schema.Attribute{
		"connector":                     s.Attributes["connector"],
		"credential.type":               credential["type"],
		"credential.client_id":          credential["client_id"],
		"credential.client_secret_name": credential["client_secret_name"],
	}
	for field, attribute := range vault.Attributes {
		fixed["credential.vault."+field] = attribute
	}
	for field, attribute := range fixed {
		if requiresReplace(t, attribute) {
			t.Errorf("%s requires replacement; want the change refused instead", field)
		}
	}
	if attribute, ok := s.Attributes["credential"].(schema.SingleNestedAttribute); !ok || len(attribute.PlanModifiers) != 0 {
		t.Errorf("credential = %+v; want a nested attribute without plan modifiers, whose change is refused instead", s.Attributes["credential"])
	}

	for _, name := range []string{"discovery", "dns_name", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
}

// P0's messages, as its own tests spell them out.
const (
	vaultAccountRule  = "Enter the 12-digit ID of the AWS account the OAuth client's secret is in."
	vaultRegionRule   = "Enter the AWS region the OAuth client's secret is in, such as us-west-2."
	vaultProjectRule  = "Enter the ID of the Google Cloud project the OAuth client's secret is in."
	secretNameRule    = "That doesn't look like a secret name. Enter the name of the secret that holds the OAuth client's secret, not its ARN, its full resource name or the client secret itself."
	secretSuffixRule  = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule = "Enter the name of the secret that holds, or will hold, the OAuth client's secret."
	clientIdRule      = "Enter the client ID of the Tailscale federated identity or OAuth client, as Tailscale's Trust credentials page shows it."
	paddedValue       = "P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. Remove the whitespace, for example with trimspace()."
)

var (
	awsVaultFields = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpVaultFields = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}
)

// A credential with fields, and with the vault built for its type, or none.
func credentialOf(fields map[string]tftypes.Value, vault func(tftypes.Object) tftypes.Value) func(tftypes.Object) tftypes.Value {
	return func(typ tftypes.Object) tftypes.Value {
		all := map[string]tftypes.Value{}
		for name, value := range fields {
			all[name] = value
		}
		if vault != nil {
			vaultType, ok := typ.AttributeTypes["vault"].(tftypes.Object)
			if !ok {
				panic("credential.vault is not an object")
			}
			all["vault"] = vault(vaultType)
		}
		return objectOf(all)(typ)
	}
}

// An OAuth client credential with the given vault, client ID and secret name.
func oauthCredential(vault func(tftypes.Object) tftypes.Value, clientId, secret tftypes.Value) func(tftypes.Object) tftypes.Value {
	return credentialOf(map[string]tftypes.Value{"type": str(oauthClientCredential), "client_id": clientId, "client_secret_name": secret}, vault)
}

// Runs the tailnet's ValidateConfig against a configuration with the given credential.
func validateTailnet(t *testing.T, credential func(tftypes.Object) tftypes.Value) diag.Diagnostics {
	t.Helper()
	blocks := map[string]func(tftypes.Object) tftypes.Value{"credential": credential}
	values := map[string]tftypes.Value{"id": str(testTailnet), "connector": str("prod")}
	return validateConfig(t, &TailscaleTailnet{}, blocks, values)
}

// P0's own cases for where an OAuth client's secret is
// (packages/integrations/tailscale/backend/src/install/__tests__/tailnet.test.ts in the
// app), plus the edges of each rule, the fields each vault takes and the fields each
// credential takes, run through ValidateConfig: each refused value gets one error, at
// its attribute, with P0's message. Each check skips a value Terraform doesn't know
// yet.
func TestTailnetValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:tailscale/acme/oauth"
	awsVault, gcpVault := objectOf(awsVaultFields), objectOf(gcpVaultFields)
	aws := func(vault func(tftypes.Object) tftypes.Value, secret string) func(tftypes.Object) tftypes.Value {
		return oauthCredential(vault, str(testClientId), str(secret))
	}
	awsSecret, gcpSecret := "tailscale/acme/oauth", "tailscale-acme-oauth"
	federated := map[string]tftypes.Value{"type": str(federatedCredential), "client_id": str(testClientId)}
	padded := func(field string) string {
		return field + ": " + strings.ReplaceAll(paddedValue, "%s", field)
	}

	cases := []struct {
		name       string
		credential func(tftypes.Object) tftypes.Value
		want       []string
	}{
		{name: "federated", credential: credentialOf(federated, nil)},
		{name: "aws", credential: aws(awsVault, awsSecret)},
		{name: "gcp", credential: aws(gcpVault, gcpSecret)},
		{name: "unknown credential", credential: unknownObject},
		{name: "unknown credential type", credential: credentialOf(map[string]tftypes.Value{"type": unknownString, "client_id": str(testClientId)}, awsVault)},
		{name: "unknown vault", credential: aws(unknownObject, awsSecret)},
		{name: "unknown vault type", credential: aws(objectOf(map[string]tftypes.Value{"type": unknownString, "account_id": str("acme")}), arn)},
		{name: "unknown secret name", credential: oauthCredential(awsVault, str(testClientId), unknownString)},
		{name: "unknown secrets region", credential: aws(with(awsVaultFields, "secrets_region", unknownString), awsSecret)},
		// The same cloud as the connector, but not the same account or project.
		{name: "a secret in another account", credential: aws(with(awsVaultFields, "account_id", str("210987654321")), awsSecret)},

		// The fields each credential takes.
		{
			name:       "an OAuth client without a vault",
			credential: oauthCredential(nil, str(testClientId), str(awsSecret)),
			want:       []string{`credential.vault: 'credential.vault' is required when 'credential.type' is "oauth-client".`},
		},
		{
			name:       "an OAuth client without a secret name",
			credential: credentialOf(map[string]tftypes.Value{"type": str(oauthClientCredential), "client_id": str(testClientId)}, awsVault),
			want:       []string{`credential.client_secret_name: 'credential.client_secret_name' is required when 'credential.type' is "oauth-client".`},
		},
		{
			name:       "a federated identity with a vault",
			credential: credentialOf(federated, awsVault),
			want:       []string{`credential.vault: 'credential.vault' may only be set when 'credential.type' is "oauth-client".`},
		},
		{
			name:       "a federated identity with a secret name",
			credential: credentialOf(map[string]tftypes.Value{"type": str(federatedCredential), "client_id": str(testClientId), "client_secret_name": str(awsSecret)}, nil),
			want:       []string{`credential.client_secret_name: 'credential.client_secret_name' may only be set when 'credential.type' is "oauth-client".`},
		},
		// A federated identity's vault isn't checked: it's refused whole.
		{
			name:       "a federated identity with a broken vault",
			credential: credentialOf(federated, with(awsVaultFields, "account_id", str("acme"))),
			want:       []string{`credential.vault: 'credential.vault' may only be set when 'credential.type' is "oauth-client".`},
		},
		{name: "a federated identity with an unknown vault", credential: credentialOf(federated, unknownObject)},

		// Where the secret is.
		{name: "an account that isn't an AWS account ID", credential: aws(with(awsVaultFields, "account_id", str("acme-secrets")), awsSecret), want: []string{"credential.vault.account_id: " + vaultAccountRule}},
		{name: "a Google Cloud region on AWS", credential: aws(with(awsVaultFields, "secrets_region", str("us-west1")), awsSecret), want: []string{"credential.vault.secrets_region: " + vaultRegionRule}},
		{name: "a GovCloud region", credential: aws(with(awsVaultFields, "secrets_region", str("us-gov-west-1")), awsSecret), want: []string{"credential.vault.secrets_region: " + unsupported}},
		{name: "a project that isn't a project ID", credential: aws(with(gcpVaultFields, "project_id", str("123456789012")), gcpSecret), want: []string{"credential.vault.project_id: " + vaultProjectRule}},

		// The fields each vault takes.
		{
			name:       "aws without an account",
			credential: aws(without(awsVaultFields, "account_id"), awsSecret),
			want:       []string{`credential.vault.account_id: 'credential.vault.account_id' is required when 'credential.vault.type' is "aws-sm".`},
		},
		{
			name:       "aws without a region",
			credential: aws(without(awsVaultFields, "secrets_region"), awsSecret),
			want:       []string{`credential.vault.secrets_region: 'credential.vault.secrets_region' is required when 'credential.vault.type' is "aws-sm".`},
		},
		{
			name:       "aws with a project",
			credential: aws(with(awsVaultFields, "project_id", str("acme-secrets")), awsSecret),
			want:       []string{`credential.vault.project_id: 'credential.vault.project_id' may only be set when 'credential.vault.type' is "gcp-sm".`},
		},
		{
			name:       "gcp without a project",
			credential: aws(without(gcpVaultFields, "project_id"), gcpSecret),
			want:       []string{`credential.vault.project_id: 'credential.vault.project_id' is required when 'credential.vault.type' is "gcp-sm".`},
		},
		{
			name:       "gcp with an account",
			credential: aws(with(gcpVaultFields, "account_id", str("123456789012")), gcpSecret),
			want:       []string{`credential.vault.account_id: 'credential.vault.account_id' may only be set when 'credential.vault.type' is "aws-sm".`},
		},
		{
			name:       "gcp with a region",
			credential: aws(with(gcpVaultFields, "secrets_region", str("us-west-2")), gcpSecret),
			want:       []string{`credential.vault.secrets_region: 'credential.vault.secrets_region' may only be set when 'credential.vault.type' is "aws-sm".`},
		},

		// The secret's name, which each secret manager has its own rule for.
		{name: "an empty name", credential: aws(awsVault, ""), want: []string{"credential.client_secret_name: " + missingSecretRule}},
		{name: "an empty name with an unknown vault", credential: aws(unknownObject, ""), want: []string{"credential.client_secret_name: " + missingSecretRule}},
		// P0 trims the name whatever the vault, so this doesn't wait for the vault's type.
		{
			name:       "a padded name with an unknown vault type",
			credential: aws(objectOf(map[string]tftypes.Value{"type": unknownString}), " tailscale/secret"),
			want:       []string{padded("credential.client_secret_name")},
		},
		{name: "an ARN", credential: aws(awsVault, arn+"-AbCdEf"), want: []string{"credential.client_secret_name: " + secretNameRule}},
		{name: "a name with a space", credential: aws(awsVault, "tailscale secret"), want: []string{"credential.client_secret_name: " + secretNameRule}},
		{name: "a wildcard", credential: aws(awsVault, "tailscale/*"), want: []string{"credential.client_secret_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", credential: aws(awsVault, "tailscale/secret-AbCdEf"), want: []string{"credential.client_secret_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", credential: aws(gcpVault, "projects/acme-secrets/secrets/tailscale-secret"), want: []string{"credential.client_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", credential: aws(gcpVault, awsSecret), want: []string{"credential.client_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", credential: aws(gcpVault, "tailscale.secret"), want: []string{"credential.client_secret_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", credential: aws(gcpVault, "tailscale-secret-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", credential: aws(awsVault, " tailscale/secret-AbCdEf\n"), want: []string{"credential.client_secret_name: " + secretSuffixRule}},
		{name: "a name with a trailing newline", credential: aws(awsVault, "tailscale/secret\n"), want: []string{padded("credential.client_secret_name")}},
		// A Tailscale secret fits either vault's rule for a name, so it's refused by its
		// prefix, in any case, whatever the vault.
		{name: "a client secret as the name", credential: aws(awsVault, testClientSecret), want: []string{"credential.client_secret_name: " + tailscaleSecretAsSecretName}},
		{name: "a client secret as a Google name", credential: aws(gcpVault, testClientSecret), want: []string{"credential.client_secret_name: " + tailscaleSecretAsSecretName}},
		{
			name:       "an upper-case secret with an unknown vault type",
			credential: aws(objectOf(map[string]tftypes.Value{"type": unknownString}), strings.ToUpper(testClientSecret)),
			want:       []string{"credential.client_secret_name: " + tailscaleSecretAsSecretName},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateTailnet(t, c.credential))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Terraform shows a nested attribute's error at its resource, so each summary says what
// the value is for. No error repeats the secret's name, because a customer may have
// pasted the OAuth client's secret in its place.
func TestTailnetValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name   string
		vault  func(tftypes.Object) tftypes.Value
		secret string
		want   []string
	}{
		{name: "an ARN", vault: objectOf(awsVaultFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid secret name for the OAuth client's secret"}},
		{name: "a full resource name", vault: objectOf(gcpVaultFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid secret name for the OAuth client's secret"}},
		{name: "a suffixed name", vault: objectOf(awsVaultFields), secret: "acme/token-Zx9Qw2", want: []string{"Secret name for the OAuth client's secret ends like an ARN suffix"}},
		{name: "a padded name", vault: objectOf(awsVaultFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", vault: objectOf(awsVaultFields), secret: "", want: []string{"Missing secret name for the OAuth client's secret"}},
		{name: "vault account", vault: with(awsVaultFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the OAuth client's secret"}},
		{name: "vault region", vault: with(awsVaultFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the OAuth client's secret"}},
		{name: "vault partition", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the OAuth client's secret"}},
		{name: "vault project", vault: with(gcpVaultFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the OAuth client's secret"}},
		{name: "a client secret", vault: objectOf(awsVaultFields), secret: testClientSecret, want: []string{"Tailscale secret entered as the secret's name"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateTailnet(t, oauthCredential(c.vault, str(testClientId), str(c.secret)))
			if got := errorSummaries(diags); !reflect.DeepEqual(got, c.want) {
				t.Errorf("errors = %v; want %v", got, c.want)
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Summary()+d.Detail(), "acme/token") || strings.Contains(d.Summary()+d.Detail(), "acme-token") ||
					strings.Contains(d.Summary()+d.Detail(), testClientSecret) {
					t.Errorf("error %v repeats the value", d)
				}
			}
		})
	}
}

// P0's own cases for the client ID
// (packages/integrations/tailscale/backend/src/install/__tests__/tailnet.test.ts in the
// app), plus the edges of the rule, run through ValidateConfig for both credentials.
// Each refused value gets one error, at credential.client_id, with P0's message, which
// doesn't repeat the value: the OAuth client's secret may be pasted in its place.
func TestTailnetValidateConfigClientId(t *testing.T) {
	cases := []struct {
		name     string
		clientId string
		want     []string
	}{
		{name: "a client ID", clientId: testClientId},
		{name: "every character a client ID can have", clientId: "aZ09_-"},
		{name: "one character", clientId: "a"},
		{name: "a client ID at its longest", clientId: strings.Repeat("a", 128)},
		{name: "empty", clientId: "", want: []string{"credential.client_id: " + clientIdRule}},
		{name: "one with a space", clientId: "abc def", want: []string{"credential.client_id: " + clientIdRule}},
		{name: "a client ID too long", clientId: strings.Repeat("a", 129), want: []string{"credential.client_id: " + clientIdRule}},
		{name: "one with a dot", clientId: "abc.def", want: []string{"credential.client_id: " + clientIdRule}},
		// P0 trims the client ID before it checks it, so a padded one is refused for its
		// whitespace, and one that's only whitespace for its form.
		{name: "a padded client ID", clientId: " " + testClientId + "\n", want: []string{"credential.client_id: " + strings.ReplaceAll(paddedValue, "%s", "credential.client_id")}},
		{name: "only whitespace", clientId: " ", want: []string{"credential.client_id: " + clientIdRule}},
		// A Tailscale secret fits the rule for a client ID, so it's refused by its prefix,
		// in any case, and even with whitespace around it.
		{name: "a client secret", clientId: testClientSecret, want: []string{"credential.client_id: " + tailscaleSecretAsClientId}},
		{name: "an upper-case prefix", clientId: "TSKEY-client-kAbc1CNTRL-Xy3q", want: []string{"credential.client_id: " + tailscaleSecretAsClientId}},
		{name: "a padded client secret", clientId: " " + testClientSecret, want: []string{"credential.client_id: " + tailscaleSecretAsClientId}},
	}

	credentials := map[string]func(clientId tftypes.Value) func(tftypes.Object) tftypes.Value{
		"federated": func(clientId tftypes.Value) func(tftypes.Object) tftypes.Value {
			return credentialOf(map[string]tftypes.Value{"type": str(federatedCredential), "client_id": clientId}, nil)
		},
		"oauth client": func(clientId tftypes.Value) func(tftypes.Object) tftypes.Value {
			return oauthCredential(objectOf(awsVaultFields), clientId, str("tailscale/acme/oauth"))
		},
	}
	for credential, build := range credentials {
		for _, c := range cases {
			t.Run(credential+", "+c.name, func(t *testing.T) {
				diags := validateTailnet(t, build(str(c.clientId)))
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

	// A client ID that Terraform doesn't know yet, such as a federated identity's that
	// the same apply creates, is checked when it is.
	if diags := validateTailnet(t, credentials["federated"](unknownString)); diags.HasError() {
		t.Errorf("errors for an unknown client ID = %v; want none", diags)
	}
}

// The credential type's validator takes the credential types the app offers.
func TestTailnetCredentialTypeValidator(t *testing.T) {
	attribute := nestedAttributes(t, schemaOf(t, NewTailscaleTailnet()), "credential")["type"]
	for value, want := range map[string]bool{"federated": true, "oauth-client": true, "oauth": false, "": false} {
		if got := accepts(t, attribute, value); got != want {
			t.Errorf("credential.type accepts %q = %v; want %v", value, got, want)
		}
	}
}

// Every field a rule names is one the vault has. A misspelt name would read a null
// value, which skips the check silently.
func TestVaultRuleFieldNames(t *testing.T) {
	names := awsVault().fields()
	vault, ok := nestedAttributes(t, schemaOf(t, NewTailscaleTailnet()), "credential")["vault"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("credential.vault is not a nested attribute")
	}
	for vaultType, fields := range vaultRules {
		for _, field := range fields {
			if _, ok := names[field.name]; !ok {
				t.Errorf("vault rule for %s names %q, which the vault doesn't have", vaultType, field.name)
			}
			if _, ok := vault.Attributes[field.name]; !ok {
				t.Errorf("vault rule for %s names %q, which the attribute doesn't have", vaultType, field.name)
			}
		}
	}
}

// Creating a tailnet stages it with every field, then verifies and configures it,
// which is when P0 validates the install and stores what discovery found.
func TestTailnetCreate(t *testing.T) {
	for _, c := range tailnetCases {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &TailscaleTailnet{}, server)

			resp := create(t, r, rawOf(t, r, c.planned()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake,
				"POST "+basePath+" {}",
				"PUT "+tailnetPath+" "+c.body,
				"POST "+tailnetPath+"/verify "+c.body,
				"POST "+tailnetPath+"/configure "+c.body,
			)
			if want := rawOf(t, r, c.tailnet(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// When the install check fails, the apply fails with the staged item in state, which
// Terraform taints. The next apply then destroys it before it creates it again, which
// is the only way back: P0 refuses to stage an item that exists.
func TestTailnetCreateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.failVerify = true
	r := configure(t, &TailscaleTailnet{}, server)

	resp := create(t, r, rawOf(t, r, federatedTailnet.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Tailscale tailnet install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+tailnetPath+" "+federatedTailnet.body,
		"POST "+tailnetPath+"/verify "+federatedTailnet.body,
	)
	if want := rawOf(t, r, federatedTailnet.tailnet(common.StateStage)); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant the staged tailnet", resp.State.Raw)
	}
}

// P0 refuses to add a tailnet twice, so one that exists is imported, not created, and
// a refused create saves nothing and leaves P0's item as it was.
func TestTailnetCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, tailnetPath, common.StateInstalled, json.RawMessage(federatedTailnet.body))
	r := configure(t, &TailscaleTailnet{}, server)

	resp := create(t, r, rawOf(t, r, federatedTailnet.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage tailnet component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+basePath+" {}", "PUT "+tailnetPath+" "+federatedTailnet.body)
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(tailnetPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

func TestTailnetDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, tailnetPath, common.StateInstalled, json.RawMessage(federatedTailnet.body))
	r := configure(t, &TailscaleTailnet{}, server)

	resp := destroy(t, r, rawOf(t, r, federatedTailnet.tailnet(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+tailnetPath)
	if got := fake.State(tailnetPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// A tailnet that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Its state, and what P0's
// discovery finds when it validates the install, are unknown in it. Every other plan is
// the framework's, so an installed tailnet plans no difference.
func TestTailnetModifyPlan(t *testing.T) {
	r := &TailscaleTailnet{}
	at := func(state string) tftypes.Value { return rawOf(t, r, federatedTailnet.tailnet(state)) }
	// The plan that finishes the install of a tailnet at state.
	finishing := rawOf(t, r, federatedTailnet.planned())
	none := rawOf(t, r, (*tailscaleTailnetModel)(nil))

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

// The plan for a tailnet changed from installed by change, as the framework makes it:
// what P0 assigns is unknown, since the configuration changed.
func changedTailnet(t *testing.T, base tailnetCase, change func(*tailscaleTailnetModel)) *tailscaleTailnetModel {
	t.Helper()
	planned := base.tailnet(common.StateInstalled)
	change(planned)
	planned.Discovery = types.StringUnknown()
	planned.DnsName = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The plan for an installed tailnet whose attribute at p Terraform only knows at apply
// time.
func withUnknown(t *testing.T, r resource.Resource, model *tailscaleTailnetModel, p path.Path, unknown any) tftypes.Value {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaOf(t, r), Raw: rawOf(t, r, model)}
	if diags := plan.SetAttribute(context.Background(), p, unknown); diags.HasError() {
		t.Fatalf("SetAttribute: %v", diags)
	}
	return plan.Raw
}

// An installed tailnet's connector and credential can't change: replacing the
// tailnet would remove it without revoking its open grants, so the plan is refused, at
// the attribute that changed. The credential is compared whole, so a vault that's
// removed with the type changed is refused too. A tailnet that isn't installed has no
// open grants, and P0 refuses the change itself; a new ID replaces the tailnet with
// another; and a value Terraform doesn't know yet is checked again on apply.
func TestTailnetModifyPlanRefusesFixedChanges(t *testing.T) {
	r := &TailscaleTailnet{}
	installed := rawOf(t, r, federatedTailnet.tailnet(common.StateInstalled))

	refused := []struct {
		name   string
		plan   *tailscaleTailnetModel
		at     string
		reason string
	}{
		{
			name:   "connector",
			plan:   changedTailnet(t, federatedTailnet, func(m *tailscaleTailnetModel) { m.Connector = types.StringValue("prod-2") }),
			at:     "connector",
			reason: "A tailnet's connector can't change",
		},
		{
			name:   "client ID",
			plan:   changedTailnet(t, federatedTailnet, func(m *tailscaleTailnetModel) { m.Credential.ClientId = types.StringValue("kXyz9CNTRL") }),
			at:     "credential",
			reason: "A tailnet's credential can't change",
		},
		{
			name:   "credential type",
			plan:   changedTailnet(t, federatedTailnet, func(m *tailscaleTailnetModel) { m.Credential = awsOAuthTailnet.credential() }),
			at:     "credential",
			reason: "A tailnet's credential can't change",
		},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			resp := modifyPlan(t, r, installed, rawOf(t, r, c.plan))
			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 || errs[0].Summary() != c.reason {
				t.Fatalf("diagnostics = %v; want %q", resp.Diagnostics, c.reason)
			}
			withPath, ok := errs[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(path.Root(c.at)) {
				t.Errorf("error at %v; want it at %s", errs[0], c.at)
			}
			if !strings.Contains(errs[0].Detail(), "revoke the tailnet's grants") {
				t.Errorf("detail = %q; want it to say how to change it", errs[0].Detail())
			}
		})
	}

	// An OAuth client whose vault goes away with its type is a change of credential, though
	// no field of the vault has a planned value to compare.
	t.Run("vault removed", func(t *testing.T) {
		prior := rawOf(t, r, awsOAuthTailnet.tailnet(common.StateInstalled))
		plan := changedTailnet(t, awsOAuthTailnet, func(m *tailscaleTailnetModel) { m.Credential = federatedTailnet.credential() })
		resp := modifyPlan(t, r, prior, rawOf(t, r, plan))
		if errs := resp.Diagnostics.Errors(); len(errs) != 1 || errs[0].Summary() != "A tailnet's credential can't change" {
			t.Errorf("diagnostics = %v; want the credential refused", resp.Diagnostics)
		}
	})

	newConnector := func(m *tailscaleTailnetModel) { m.Connector = types.StringValue("prod-2") }
	staged := federatedTailnet.tailnet(common.StateStage)
	allowed := []struct {
		name  string
		prior tftypes.Value
		plan  tftypes.Value
	}{
		{name: "a tailnet at stage", prior: rawOf(t, r, staged), plan: rawOf(t, r, changedTailnet(t, federatedTailnet, newConnector))},
		{
			name:  "a new id",
			prior: installed,
			plan: rawOf(t, r, changedTailnet(t, federatedTailnet, func(m *tailscaleTailnetModel) {
				m.Id = types.StringValue("T5678CNTRL")
				newConnector(m)
			})),
		},
		{
			name:  "an unknown client ID",
			prior: installed,
			plan:  withUnknown(t, r, changedTailnet(t, federatedTailnet, func(*tailscaleTailnetModel) {}), credentialPath.AtName("client_id"), types.StringUnknown()),
		},
		{
			name:  "an unknown connector",
			prior: installed,
			plan:  withUnknown(t, r, changedTailnet(t, federatedTailnet, func(*tailscaleTailnetModel) {}), path.Root("connector"), types.StringUnknown()),
		},
	}
	for _, c := range allowed {
		t.Run(c.name, func(t *testing.T) {
			if resp := modifyPlan(t, r, c.prior, c.plan); resp.Diagnostics.HasError() {
				t.Errorf("diagnostics = %v; want the plan allowed", resp.Diagnostics)
			}
		})
	}
}

// A change that Terraform only knew at apply time is refused on apply, and sends
// nothing to P0.
func TestTailnetUpdateRefusesFixedChanges(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, tailnetPath, common.StateInstalled, json.RawMessage(federatedTailnet.body))
	r := configure(t, &TailscaleTailnet{}, server)
	prior := rawOf(t, r, federatedTailnet.tailnet(common.StateInstalled))
	planned := changedTailnet(t, federatedTailnet, func(m *tailscaleTailnetModel) { m.Credential.ClientId = types.StringValue("kXyz9CNTRL") })

	resp := update(t, r, prior, rawOf(t, r, planned))

	if errs := resp.Diagnostics.Errors(); len(errs) != 1 || errs[0].Summary() != "A tailnet's credential can't change" {
		t.Fatalf("diagnostics = %v; want the credential refused", resp.Diagnostics)
	}
	wantRequests(t, fake)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want it unchanged", resp.State.Raw)
	}
}

// A tailnet imported at stage converges in one apply: its plan shows an update, the
// update verifies and configures it without staging it, and the plan after that shows
// no difference.
func TestTailnetStagedItemConverges(t *testing.T) {
	for _, c := range tailnetCases {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, tailnetPath, common.StateStage, json.RawMessage(c.body))
			r := configure(t, &TailscaleTailnet{}, server)

			staged := rawOf(t, r, c.tailnet(common.StateStage))
			plan := modifyPlan(t, r, staged, staged)
			if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
				t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
			}

			resp := update(t, r, staged, plan.Plan.Raw)

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+tailnetPath+"/verify "+c.body, "POST "+tailnetPath+"/configure "+c.body)
			installed := rawOf(t, r, c.tailnet(common.StateInstalled))
			if !resp.State.Raw.Equal(installed) {
				t.Fatalf("state =\n%v\nwant the installed tailnet", resp.State.Raw)
			}
			if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
				t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
			}
		})
	}
}

// When the check fails on the update that finishes an install, the apply fails and the
// state keeps the item as it was, so the next plan shows the update again.
func TestTailnetUpdateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, tailnetPath, common.StateStage, json.RawMessage(federatedTailnet.body))
	fake.failVerify = true
	r := configure(t, &TailscaleTailnet{}, server)
	prior := rawOf(t, r, federatedTailnet.tailnet(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, federatedTailnet.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Tailscale tailnet install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+tailnetPath+"/verify "+federatedTailnet.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged tailnet kept", resp.State.Raw)
	}
}

// An import ID goes into the read's URL path, so one that isn't a Tailnet ID is refused
// before the read, with P0's message.
func TestTailnetImportState(t *testing.T) {
	r := &TailscaleTailnet{}
	s := schemaOf(t, r)
	for id, want := range map[string]bool{testTailnet: true, "example.com": true, "tail1234.ts.net": false, "T1234#x": false, "T1234%zz": false} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, resp)
		if got := !resp.Diagnostics.HasError(); got != want {
			t.Errorf("import %q accepted = %v; want %v (%v)", id, got, want, resp.Diagnostics)
		}
		if !want && (len(resp.Diagnostics.Errors()) != 1 || resp.Diagnostics.Errors()[0].Detail() != invalidTailnetId) {
			t.Errorf("import %q refused with %v; want P0's message", id, resp.Diagnostics)
		}
	}
}
