package installonepassword

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// An account whose token is in one cloud's secret manager, as Terraform and as
// P0's API hold it.
type accountCloud struct {
	name          string
	secretManager func() *secretManagerModel
	// The name of the secret that holds the token.
	secret string
	// The body of every request that carries the account: its connector, its secret
	// manager, its token's secret, its one-time links switch, off by default, and its
	// request limit, item by default.
	body string
}

var awsAccount = accountCloud{
	name:          "aws",
	secretManager: awsSM,
	secret:        "1password/engineering/token",
	body: `{"connector":"prod",` +
		`"secretManager":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"},` +
		`"tokenSecretName":"1password/engineering/token","oneTimeLinks":false,"requestLimit":{"type":"item"}}`,
}

var gcpAccount = accountCloud{
	name:          "gcp",
	secretManager: gcpSM,
	secret:        "1password-engineering-token",
	body: `{"connector":"prod",` +
		`"secretManager":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"tokenSecretName":"1password-engineering-token","oneTimeLinks":false,"requestLimit":{"type":"item"}}`,
}

var accountClouds = []accountCloud{awsAccount, gcpAccount}

// The account at state, with the allowed domains that P0 reads back once it has
// validated the install.
func (c accountCloud) account(state string) *onePasswordAccountModel {
	domains := types.StringNull()
	if state != common.StateStage {
		domains = types.StringValue(fakeAllowedDomains)
	}
	return &onePasswordAccountModel{
		Id:              types.StringValue("acme.1password.com"),
		Connector:       types.StringValue("prod"),
		SecretManager:   c.secretManager(),
		TokenSecretName: types.StringValue(c.secret),
		OneTimeLinks:    types.BoolValue(false),
		RequestLimit:    types.StringValue("item"),
		AllowedDomains:  domains,
		State:           types.StringValue(state),
	}
}

// The plan for a new account: everything P0 assigns is unknown.
func (c accountCloud) planned() *onePasswordAccountModel {
	planned := c.account(common.StateStage)
	planned.AllowedDomains = types.StringUnknown()
	planned.State = types.StringUnknown()
	return planned
}

// The account at state with one-time links on, and the body that carries it.
func (c accountCloud) withOneTimeLinks(state string) (*onePasswordAccountModel, string) {
	model := c.account(state)
	model.OneTimeLinks = types.BoolValue(true)
	return model, strings.Replace(c.body, `"oneTimeLinks":false`, `"oneTimeLinks":true`, 1)
}

// The account at state with the request limit at limit, and the body that carries it.
func (c accountCloud) withRequestLimit(state, limit string) (*onePasswordAccountModel, string) {
	model := c.account(state)
	model.RequestLimit = types.StringValue(limit)
	return model, strings.Replace(c.body, `"requestLimit":{"type":"item"}`, `"requestLimit":{"type":"`+limit+`"}`, 1)
}

// The item as P0's API answers at state.
func (c accountCloud) response(state string) string {
	domains := ""
	if state != common.StateStage {
		domains = `,"allowedDomains":"^example\\.com$"`
	}
	return c.body[:len(c.body)-1] + domains + `,"state":"` + state + `"}`
}

// The PUT that stages the item, and verify and configure, carry the account's
// connector, secret manager and secret, and its one-time links and request limit, in
// the field names of the app's install schema: `secretManager` is the shared vault
// element, in which "install" names the AWS account or the project, and
// `requestLimit` is a select. None carries what P0 assigns.
func TestAccountRequestJson(t *testing.T) {
	for _, c := range accountClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&OnePasswordAccount{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&OnePasswordAccount{}).toJson(c.account(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed account =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in: the state and, once it has validated the install, the allowed domains.
func TestAccountFromJson(t *testing.T) {
	for _, c := range accountClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item onePasswordAccountJson
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &item).(*onePasswordAccountModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.account(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				// The model is one Terraform's state takes.
				rawOf(t, NewOnePasswordAccount(), got)
			})
		}
	}
}

func TestAccountFromJsonRequiresSecretManager(t *testing.T) {
	var diags diag.Diagnostics
	got := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &onePasswordAccountJson{})
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
		t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
	}
}

// A secret manager of a type this provider doesn't know is refused rather than read
// into the wrong fields.
func TestAccountFromJsonRequiresAKnownSecretManager(t *testing.T) {
	item := onePasswordAccountJson{SecretManager: &secretManagerJson{Type: "azure-kv", Install: "acme"}}

	var diags diag.Diagnostics
	got := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &item)
	if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported 1Password account" {
		t.Errorf("fromJson = %+v, %v; want nil and an unsupported account error", got, diags)
	}
	// The error names the account, its secret manager and the secret managers this
	// provider takes.
	const want = `The 1Password account acme.1password.com has a "azure-kv" secret manager. This provider supports only aws-sm and gcp-sm.`
	for _, d := range diags.Errors() {
		if !strings.HasPrefix(d.Detail(), want) {
			t.Errorf("error = %q; want it to start with %q", d.Detail(), want)
		}
	}
}

// P0 fixes the connector, the secret manager and the token's secret when it creates
// the item, so changing any of them replaces the installation. What P0 assigns is
// read-only.
func TestAccountSchema(t *testing.T) {
	s := schemaOf(t, NewOnePasswordAccount())
	secretManager := nestedAttributes(t, s, "secret_manager")
	for field, attribute := range map[string]schema.Attribute{
		"id":                            s.Attributes["id"],
		"connector":                     s.Attributes["connector"],
		"token_secret_name":             s.Attributes["token_secret_name"],
		"secret_manager.type":           secretManager["type"],
		"secret_manager.account_id":     secretManager["account_id"],
		"secret_manager.project_id":     secretManager["project_id"],
		"secret_manager.secrets_region": secretManager["secrets_region"],
	} {
		if !requiresReplace(t, attribute) {
			t.Errorf("%s doesn't require replacement", field)
		}
	}

	for _, name := range []string{"allowed_domains", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}

	// P0 lets one_time_links change after install, so it changes in place, and it is
	// off unless the configuration turns it on.
	oneTimeLinks, ok := s.Attributes["one_time_links"].(schema.BoolAttribute)
	if !ok || !oneTimeLinks.Optional || !oneTimeLinks.Computed || len(oneTimeLinks.PlanModifiers) != 0 {
		t.Errorf("one_time_links = %+v; want an optional, computed attribute that changes in place", s.Attributes["one_time_links"])
	}
	if oneTimeLinks.Default == nil {
		t.Fatal("one_time_links has no default")
	}
	var def defaults.BoolResponse
	oneTimeLinks.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, &def)
	if !def.PlanValue.Equal(types.BoolValue(false)) {
		t.Errorf("one_time_links default = %v; want false", def.PlanValue)
	}

	// P0 lets request_limit change after install too, and it is one per item unless
	// the configuration says otherwise.
	requestLimit, ok := s.Attributes["request_limit"].(schema.StringAttribute)
	if !ok || !requestLimit.Optional || !requestLimit.Computed || len(requestLimit.PlanModifiers) != 0 {
		t.Errorf("request_limit = %+v; want an optional, computed attribute that changes in place", s.Attributes["request_limit"])
	}
	if requestLimit.Default == nil {
		t.Fatal("request_limit has no default")
	}
	var limit defaults.StringResponse
	requestLimit.Default.DefaultString(context.Background(), defaults.StringRequest{}, &limit)
	if !limit.PlanValue.Equal(types.StringValue("item")) {
		t.Errorf("request_limit default = %v; want item", limit.PlanValue)
	}
	for value, want := range map[string]bool{"user": true, "vault": true, "item": true, "": false, "Item": false, "account": false, " item": false} {
		if got := accepts(t, requestLimit, value); got != want {
			t.Errorf("request_limit accepts %q = %v; want %v", value, got, want)
		}
	}
}

// P0's messages, as its own tests spell them out.
const (
	secretManagerAccountRule = "Enter the 12-digit ID of the AWS account the token's secret is in."
	secretManagerRegionRule  = "Enter the AWS region the token's secret is in, such as us-west-2."
	secretManagerProjectRule = "Enter the ID of the Google Cloud project the token's secret is in."
	secretNameRule           = "That doesn't look like a secret name. Enter the name of the secret that holds the service account token, not its ARN, its full resource name or the token itself."
	pastedTokenRule          = "That's a 1Password service account token, not the name of its secret. Enter the name of the secret that holds the token. P0 never needs the token itself, so revoke it and create a new one if it was shared anywhere else."
	secretSuffixRule         = "Secrets Manager can't find a secret by its name when the name ends in a hyphen and six characters, like -AbCdEf. Use a secret whose name doesn't end that way."
	missingSecretRule        = "Enter the name of the secret that holds, or will hold, the account's service account token."
)

var (
	awsSMFields = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-west-2")}
	gcpSMFields = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("acme-secrets")}
)

// Runs the account's ValidateConfig against a configuration with the given secret
// manager and secret name.
func validateAccount(t *testing.T, secretManager func(tftypes.Object) tftypes.Value, secret tftypes.Value) diag.Diagnostics {
	t.Helper()
	blocks := map[string]func(tftypes.Object) tftypes.Value{"secret_manager": secretManager}
	values := map[string]tftypes.Value{
		"id":                str("acme.1password.com"),
		"connector":         str("prod"),
		"token_secret_name": secret,
	}
	return validateConfig(t, &OnePasswordAccount{}, blocks, values)
}

// P0's own cases for where the token's secret is
// (packages/integrations/one-password/backend/src/install/__tests__ in the app), plus the edges of each rule and the fields each secret manager takes, run
// through ValidateConfig: each refused value gets one error, at its attribute, with P0's
// message. Each check skips a value Terraform doesn't know yet.
func TestAccountValidateConfig(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-west-2:123456789012:secret:1password/engineering/token"
	awsSM, gcpSM := objectOf(awsSMFields), objectOf(gcpSMFields)
	awsSecret, gcpSecret := str(awsAccount.secret), str(gcpAccount.secret)

	cases := []struct {
		name          string
		secretManager func(tftypes.Object) tftypes.Value
		secret        tftypes.Value
		want          []string
	}{
		{name: "aws", secretManager: awsSM, secret: awsSecret},
		{name: "gcp", secretManager: gcpSM, secret: gcpSecret},
		{name: "unknown secret manager", secretManager: unknownObject, secret: awsSecret},
		{name: "unknown secret manager type", secretManager: objectOf(map[string]tftypes.Value{"type": unknownString, "account_id": str("acme")}), secret: str(arn)},
		{name: "unknown secret name", secretManager: awsSM, secret: unknownString},
		{name: "unknown secrets region", secretManager: with(awsSMFields, "secrets_region", unknownString), secret: awsSecret},
		// The same cloud as the connector, but not the same account or project.
		{name: "a secret in another account", secretManager: with(awsSMFields, "account_id", str("210987654321")), secret: awsSecret},

		// Where the secret is.
		{name: "an account that isn't an AWS account ID", secretManager: with(awsSMFields, "account_id", str("acme-secrets")), secret: awsSecret, want: []string{"secret_manager.account_id: " + secretManagerAccountRule}},
		{name: "a Google Cloud region on AWS", secretManager: with(awsSMFields, "secrets_region", str("us-west1")), secret: awsSecret, want: []string{"secret_manager.secrets_region: " + secretManagerRegionRule}},
		{name: "a GovCloud region", secretManager: with(awsSMFields, "secrets_region", str("us-gov-west-1")), secret: awsSecret, want: []string{"secret_manager.secrets_region: " + unsupported}},
		{name: "a project that isn't a project ID", secretManager: with(gcpSMFields, "project_id", str("123456789012")), secret: gcpSecret, want: []string{"secret_manager.project_id: " + secretManagerProjectRule}},

		// The fields each secret manager takes.
		{name: "aws without an account", secretManager: without(awsSMFields, "account_id"), secret: awsSecret, want: []string{`secret_manager.account_id: 'secret_manager.account_id' is required when 'secret_manager.type' is "aws-sm".`}},
		{name: "aws without a region", secretManager: without(awsSMFields, "secrets_region"), secret: awsSecret, want: []string{`secret_manager.secrets_region: 'secret_manager.secrets_region' is required when 'secret_manager.type' is "aws-sm".`}},
		{
			name:          "aws with a project",
			secretManager: with(awsSMFields, "project_id", str("acme-secrets")),
			secret:        awsSecret,
			want:          []string{`secret_manager.project_id: 'secret_manager.project_id' may only be set when 'secret_manager.type' is "gcp-sm".`},
		},
		{name: "gcp without a project", secretManager: without(gcpSMFields, "project_id"), secret: gcpSecret, want: []string{`secret_manager.project_id: 'secret_manager.project_id' is required when 'secret_manager.type' is "gcp-sm".`}},
		{
			name:          "gcp with an account",
			secretManager: with(gcpSMFields, "account_id", str("123456789012")),
			secret:        gcpSecret,
			want:          []string{`secret_manager.account_id: 'secret_manager.account_id' may only be set when 'secret_manager.type' is "aws-sm".`},
		},
		{
			name:          "gcp with a region",
			secretManager: with(gcpSMFields, "secrets_region", str("us-west-2")),
			secret:        gcpSecret,
			want:          []string{`secret_manager.secrets_region: 'secret_manager.secrets_region' may only be set when 'secret_manager.type' is "aws-sm".`},
		},

		// The secret's name, which each secret manager has its own rule for.
		{name: "an empty name", secretManager: awsSM, secret: str(""), want: []string{"token_secret_name: " + missingSecretRule}},
		{name: "an empty name with an unknown secret manager", secretManager: unknownObject, secret: str(""), want: []string{"token_secret_name: " + missingSecretRule}},
		// P0 trims the name whatever the secret manager, so this doesn't wait for the
		// secret manager's type.
		{
			name:          "a padded name with an unknown secret manager type",
			secretManager: objectOf(map[string]tftypes.Value{"type": unknownString}),
			secret:        str(" 1password/token"),
			want: []string{"token_secret_name: P0 removes the whitespace around 'token_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "an ARN", secretManager: awsSM, secret: str(arn + "-AbCdEf"), want: []string{"token_secret_name: " + secretNameRule}},
		{name: "a name with a space", secretManager: awsSM, secret: str("1password token"), want: []string{"token_secret_name: " + secretNameRule}},
		{name: "a wildcard", secretManager: awsSM, secret: str("1password/*"), want: []string{"token_secret_name: " + secretNameRule}},
		{name: "a name that ends like an ARN", secretManager: awsSM, secret: str("1password/token-AbCdEf"), want: []string{"token_secret_name: " + secretSuffixRule}},
		{name: "a Google secret's full resource name", secretManager: gcpSM, secret: str("projects/acme-secrets/secrets/1password-token"), want: []string{"token_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a slash", secretManager: gcpSM, secret: awsSecret, want: []string{"token_secret_name: " + secretNameRule}},
		{name: "a Google secret's name with a dot", secretManager: gcpSM, secret: str("1password.token"), want: []string{"token_secret_name: " + secretNameRule}},
		// A Google secret's ID can end the way an AWS ARN does.
		{name: "a Google secret's name that ends like an ARN", secretManager: gcpSM, secret: str("1password-token-AbCdEf")},
		// One error for the field, for the first rule the trimmed name breaks.
		{name: "a padded name that ends like an ARN", secretManager: awsSM, secret: str(" 1password/token-AbCdEf\n"), want: []string{"token_secret_name: " + secretSuffixRule}},
		{
			name:          "a name with a trailing newline",
			secretManager: awsSM,
			secret:        str("1password/token\n"),
			want: []string{"token_secret_name: P0 removes the whitespace around 'token_secret_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateAccount(t, c.secretManager, c.secret))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Terraform shows a nested attribute's error at its resource, so each summary says what
// the value is for. No error repeats the secret's name: a token may be pasted in its
// place.
func TestAccountValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name          string
		secretManager func(tftypes.Object) tftypes.Value
		secret        string
		want          []string
	}{
		{name: "an ARN", secretManager: objectOf(awsSMFields), secret: "arn:aws:secretsmanager:us-west-2:123456789012:secret:acme/token-Zx9Qw2", want: []string{"Invalid service account token secret name"}},
		{name: "a full resource name", secretManager: objectOf(gcpSMFields), secret: "projects/acme-secrets/secrets/acme-token", want: []string{"Invalid service account token secret name"}},
		{name: "a suffixed name", secretManager: objectOf(awsSMFields), secret: "acme/token-Zx9Qw2", want: []string{"Service account token secret name ends like an ARN suffix"}},
		{name: "a padded name", secretManager: objectOf(awsSMFields), secret: " acme/token", want: []string{"Whitespace around a value"}},
		{name: "an empty name", secretManager: objectOf(awsSMFields), secret: "", want: []string{"Missing service account token secret name"}},
		{name: "secret manager account", secretManager: with(awsSMFields, "account_id", str("acme")), secret: "acme/token", want: []string{"Invalid AWS account ID for the service account token's secret"}},
		{name: "secret manager region", secretManager: with(awsSMFields, "secrets_region", str("us-west")), secret: "acme/token", want: []string{"Invalid AWS region for the service account token's secret"}},
		{name: "secret manager partition", secretManager: with(awsSMFields, "secrets_region", str("cn-north-1")), secret: "acme/token", want: []string{"Unsupported AWS region for the service account token's secret"}},
		{name: "secret manager project", secretManager: with(gcpSMFields, "project_id", str("acme")), secret: "acme-token", want: []string{"Invalid Google Cloud project for the service account token's secret"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateAccount(t, c.secretManager, str(c.secret))
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

// A 1Password service account token pasted in place of its secret's name is refused
// for either secret manager, and while Terraform doesn't know the secret manager or its
// type yet. A token fits both secret managers' rules for a name, so one that got past
// here would show in the plan and reach P0. The error doesn't repeat it.
func TestAccountValidateConfigRefusesAPastedToken(t *testing.T) {
	secretManagers := map[string]func(tftypes.Object) tftypes.Value{
		"aws":                         objectOf(awsSMFields),
		"gcp":                         objectOf(gcpSMFields),
		"unknown secret manager type": objectOf(map[string]tftypes.Value{"type": unknownString}),
		"unknown secret manager":      unknownObject,
	}
	tokens := map[string]string{
		"a service account token": "ops_eyJzaWduSW5BZGRyZXNzIjoiYWNtZS4xcGFzc3dvcmQuY29tIn0",
		"a short token":           "ops_x",
		"a token's prefix alone":  "ops_",
		"a padded token":          " ops_eyJzaWduSW5BZGRyZXNzIjoiYWNtZS4xcGFzc3dvcmQuY29tIn0\n",
	}

	for secretManagerName, secretManager := range secretManagers {
		for tokenName, token := range tokens {
			t.Run(tokenName+", "+secretManagerName, func(t *testing.T) {
				diags := validateAccount(t, secretManager, str(token))
				if got, want := errorDetails(diags), []string{"token_secret_name: " + pastedTokenRule}; !reflect.DeepEqual(got, want) {
					t.Errorf("errors =\n%v\nwant\n%v", got, want)
				}
				for _, d := range diags {
					if strings.Contains(d.Summary()+d.Detail(), strings.TrimSpace(token)) {
						t.Errorf("error %v repeats the token", d)
					}
				}
			})
		}
	}
}

// Every field a rule names is one the secret manager has. A misspelt name would read a
// null value, which skips the check silently.
func TestSecretManagerRuleFieldNames(t *testing.T) {
	names := awsAccount.secretManager().fields()
	attributes := nestedAttributes(t, schemaOf(t, NewOnePasswordAccount()), "secret_manager")
	for secretManagerType, fields := range secretManagerRules {
		for _, field := range fields {
			if _, ok := names[field.name]; !ok {
				t.Errorf("secret manager rule for %s names %q, which the secret manager doesn't have", secretManagerType, field.name)
			}
			if _, ok := attributes[field.name]; !ok {
				t.Errorf("secret manager rule for %s names %q, which the attribute doesn't have", secretManagerType, field.name)
			}
		}
	}
}

// Creating an account stages it with every field, then verifies and configures
// it, which is when P0 validates the install and reads the allowed domains back.
func TestAccountCreate(t *testing.T) {
	for _, c := range accountClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &OnePasswordAccount{}, server)

			resp := create(t, r, rawOf(t, r, c.planned()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake,
				"POST "+basePath+" {}",
				"PUT "+accountPath+" "+c.body,
				"POST "+accountPath+"/verify "+c.body,
				"POST "+accountPath+"/configure "+c.body,
			)
			if want := rawOf(t, r, c.account(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// When the install check fails, the apply fails with the staged item in state, which
// Terraform taints. The next apply then destroys it before it creates it again, which
// is the only way back: P0 refuses to stage an item that exists.
func TestAccountCreateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.failVerify = true
	r := configure(t, &OnePasswordAccount{}, server)

	resp := create(t, r, rawOf(t, r, awsAccount.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "1Password account install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+accountPath+" "+awsAccount.body,
		"POST "+accountPath+"/verify "+awsAccount.body,
	)
	if want := rawOf(t, r, awsAccount.account(common.StateStage)); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant the staged account", resp.State.Raw)
	}
}

// P0 refuses to add an account twice, so one that exists is imported, not
// created, and a refused create saves nothing and leaves P0's item as it was.
func TestAccountCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, accountPath, common.StateInstalled, json.RawMessage(awsAccount.body))
	r := configure(t, &OnePasswordAccount{}, server)

	resp := create(t, r, rawOf(t, r, awsAccount.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage account component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+basePath+" {}", "PUT "+accountPath+" "+awsAccount.body)
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(accountPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

func TestAccountDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, accountPath, common.StateInstalled, json.RawMessage(awsAccount.body))
	r := configure(t, &OnePasswordAccount{}, server)

	resp := destroy(t, r, rawOf(t, r, awsAccount.account(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+accountPath)
	if got := fake.State(accountPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// An account that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Its state, and the
// allowed domains that P0 reads back when it validates the install, are unknown in it.
// Every other plan is the framework's, so an installed account plans no difference.
func TestAccountModifyPlan(t *testing.T) {
	r := &OnePasswordAccount{}
	at := func(state string) tftypes.Value { return rawOf(t, r, awsAccount.account(state)) }
	// The plan that finishes the install of an account at state.
	finishing := rawOf(t, r, awsAccount.planned())
	none := rawOf(t, r, (*onePasswordAccountModel)(nil))

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

// An account imported at stage converges in one apply: its plan shows an update,
// the update verifies and configures it without staging it, and the plan after that
// shows no difference.
func TestAccountStagedItemConverges(t *testing.T) {
	for _, c := range accountClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, accountPath, common.StateStage, json.RawMessage(c.body))
			r := configure(t, &OnePasswordAccount{}, server)

			staged := rawOf(t, r, c.account(common.StateStage))
			plan := modifyPlan(t, r, staged, staged)
			if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
				t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
			}

			resp := update(t, r, staged, plan.Plan.Raw)

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+accountPath+"/verify "+c.body, "POST "+accountPath+"/configure "+c.body)
			installed := rawOf(t, r, c.account(common.StateInstalled))
			if !resp.State.Raw.Equal(installed) {
				t.Fatalf("state =\n%v\nwant the installed account", resp.State.Raw)
			}
			if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
				t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
			}
		})
	}
}

// When the check fails on the update that finishes an install, the apply fails and the
// state keeps the item as it was, so the next plan shows the update again.
func TestAccountUpdateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, accountPath, common.StateStage, json.RawMessage(awsAccount.body))
	fake.failVerify = true
	r := configure(t, &OnePasswordAccount{}, server)
	prior := rawOf(t, r, awsAccount.account(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsAccount.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "1Password account install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+accountPath+"/verify "+awsAccount.body)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the staged account kept", resp.State.Raw)
	}
}

// Creating an account with one-time links on sends the switch on every step, and reads
// it back.
func TestAccountCreateWithOneTimeLinks(t *testing.T) {
	fake, server := newFakeItems(t)
	r := configure(t, &OnePasswordAccount{}, server)
	installed, body := awsAccount.withOneTimeLinks(common.StateInstalled)
	planned := awsAccount.planned()
	planned.OneTimeLinks = types.BoolValue(true)

	resp := create(t, r, rawOf(t, r, planned))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+accountPath+" "+body,
		"POST "+accountPath+"/verify "+body,
		"POST "+accountPath+"/configure "+body,
	)
	if want := rawOf(t, r, installed); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
	}
}

// P0 lets one-time links change after install, so changing them on an account P0 has
// verified updates it with the configure step alone, without calling the connector
// to validate the install again. Both directions converge.
func TestAccountUpdateOneTimeLinks(t *testing.T) {
	for _, from := range []string{common.StateConfigure, common.StateInstalled} {
		t.Run("at "+from, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, accountPath, from, json.RawMessage(awsAccount.body))
			r := configure(t, &OnePasswordAccount{}, server)
			on, onBody := awsAccount.withOneTimeLinks(common.StateInstalled)

			planned := *on
			planned.AllowedDomains = types.StringUnknown()
			planned.State = types.StringUnknown()
			resp := update(t, r, rawOf(t, r, awsAccount.account(from)), rawOf(t, r, &planned))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+accountPath+"/configure "+onBody)
			if want := rawOf(t, r, on); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}

	t.Run("off again", func(t *testing.T) {
		fake, server := newFakeItems(t)
		on, onBody := awsAccount.withOneTimeLinks(common.StateInstalled)
		fake.seed(t, accountPath, common.StateInstalled, json.RawMessage(onBody))
		r := configure(t, &OnePasswordAccount{}, server)

		resp := update(t, r, rawOf(t, r, on), rawOf(t, r, awsAccount.account(common.StateInstalled)))

		if resp.Diagnostics.HasError() {
			t.Fatalf("Update: %v", resp.Diagnostics)
		}
		wantRequests(t, fake, "POST "+accountPath+"/configure "+awsAccount.body)
		if want := rawOf(t, r, awsAccount.account(common.StateInstalled)); !resp.State.Raw.Equal(want) {
			t.Errorf("state =\n%v\nwant the account with one-time links off", resp.State.Raw)
		}
	})
}

// An account P0 stored before it had one-time links reads as off, as P0 treats it, so it
// plans no difference against a configuration that leaves them out.
func TestAccountFromJsonWithoutOneTimeLinks(t *testing.T) {
	var item onePasswordAccountJson
	body := strings.Replace(awsAccount.response(common.StateInstalled), `"oneTimeLinks":false,`, "", 1)
	if err := json.Unmarshal([]byte(body), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if item.OneTimeLinks != nil {
		t.Fatalf("response %s still carries oneTimeLinks", body)
	}

	var diags diag.Diagnostics
	got, ok := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &item).(*onePasswordAccountModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson failed: %v", diags)
	}
	if want := awsAccount.account(common.StateInstalled); !reflect.DeepEqual(got, want) {
		t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
	}
}

// An account P0 stored without a request limit reads as one per item, the default, so
// it plans no difference against a configuration that leaves it out.
func TestAccountFromJsonWithoutRequestLimit(t *testing.T) {
	var item onePasswordAccountJson
	body := strings.Replace(awsAccount.response(common.StateInstalled), `,"requestLimit":{"type":"item"}`, "", 1)
	if err := json.Unmarshal([]byte(body), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if item.RequestLimit != nil {
		t.Fatalf("response %s still carries requestLimit", body)
	}

	var diags diag.Diagnostics
	got, ok := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &item).(*onePasswordAccountModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson failed: %v", diags)
	}
	if want := awsAccount.account(common.StateInstalled); !reflect.DeepEqual(got, want) {
		t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
	}
}

// Each request limit is sent as the select P0 stores, and read back.
func TestAccountRequestLimitJson(t *testing.T) {
	for _, limit := range requestLimits {
		t.Run(limit, func(t *testing.T) {
			model, body := awsAccount.withRequestLimit(common.StateInstalled, limit)
			if got := marshal(t, (&OnePasswordAccount{}).toJson(model)); got != body {
				t.Errorf("request body =\n%s\nwant\n%s", got, body)
			}

			var item onePasswordAccountJson
			if err := json.Unmarshal([]byte(body), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			item.AllowedDomains = model.AllowedDomains.ValueStringPointer()
			item.State = model.State.ValueStringPointer()
			var diags diag.Diagnostics
			got, ok := (&OnePasswordAccount{}).fromJson(context.Background(), &diags, "acme.1password.com", &item).(*onePasswordAccountModel)
			if !ok || diags.HasError() {
				t.Fatalf("fromJson failed: %v", diags)
			}
			if !reflect.DeepEqual(got, model) {
				t.Errorf("fromJson =\n%+v\nwant\n%+v", got, model)
			}
		})
	}
}

// P0 lets the request limit change after install, so changing it on an account P0 has
// verified updates it with the configure step alone, as one-time links do.
func TestAccountUpdateRequestLimit(t *testing.T) {
	for _, from := range []string{common.StateConfigure, common.StateInstalled} {
		t.Run("at "+from, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, accountPath, from, json.RawMessage(awsAccount.body))
			r := configure(t, &OnePasswordAccount{}, server)
			perVault, body := awsAccount.withRequestLimit(common.StateInstalled, "vault")

			planned := *perVault
			planned.AllowedDomains = types.StringUnknown()
			planned.State = types.StringUnknown()
			resp := update(t, r, rawOf(t, r, awsAccount.account(from)), rawOf(t, r, &planned))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+accountPath+"/configure "+body)
			if want := rawOf(t, r, perVault); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}
