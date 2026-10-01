package installgithubrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

func awsModel() *repositoryAccessModel {
	return &repositoryAccessModel{
		Org:   types.StringValue("my-github-org"),
		AppId: types.StringValue("123456"),
		Vault: &vaultModel{
			Type:          types.StringValue(AwsSecretsManager),
			AccountId:     types.StringValue("123456789012"),
			ProjectId:     types.StringNull(),
			SecretsRegion: types.StringValue("us-east-1"),
		},
		PrivateKeySecretName: types.StringValue("github/my-github-org/private-key"),
		Hosting: &installapp.ConnectorHostingModel{
			Type:                types.StringValue(installapp.AwsHosting),
			AccountId:           types.StringValue("123456789012"),
			ProjectId:           types.StringNull(),
			ConnectorName:       types.StringValue("p0-github-repositories-connector"),
			ConnectorRegion:     types.StringValue("us-east-1"),
			ConnectorServiceUri: types.StringNull(),
		},
		State: types.StringNull(),
	}
}

func gcpModel() *repositoryAccessModel {
	return &repositoryAccessModel{
		Org:   types.StringValue("my-github-org"),
		AppId: types.StringValue("234567"),
		Vault: &vaultModel{
			Type:          types.StringValue(GcpSecretManager),
			AccountId:     types.StringNull(),
			ProjectId:     types.StringValue("my-project-id"),
			SecretsRegion: types.StringNull(),
		},
		PrivateKeySecretName: types.StringValue("github-private-key"),
		Hosting: &installapp.ConnectorHostingModel{
			Type:                types.StringValue(installapp.GcpHosting),
			AccountId:           types.StringNull(),
			ProjectId:           types.StringValue("my-project-id"),
			ConnectorName:       types.StringValue("p0-github-repositories-connector"),
			ConnectorRegion:     types.StringValue("us-central1"),
			ConnectorServiceUri: types.StringNull(),
		},
		State: types.StringNull(),
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

// The PUT that stages the item carries the whole item, in the field names of the app's
// install schema: `vault` is the shared Vault element and `hosting` is the app
// integration's ConnectorHosting element.
func TestRepositoryAccessStageJson(t *testing.T) {
	cases := map[string]struct {
		model *repositoryAccessModel
		want  string
	}{
		"aws": {
			model: awsModel(),
			want: `{"appId":"123456",` +
				`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-east-1"},` +
				`"privateKeySecretName":"github/my-github-org/private-key",` +
				`"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-github-repositories-connector","connectorRegion":"us-east-1"}}`,
		},
		"gcp": {
			model: gcpModel(),
			want: `{"appId":"234567",` +
				`"vault":{"type":"gcp-sm","install":"my-project-id"},` +
				`"privateKeySecretName":"github-private-key",` +
				`"hosting":{"type":"gcp","projectId":"my-project-id","connectorName":"p0-github-repositories-connector","connectorRegion":"us-central1"}}`,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := marshal(t, stageJson(c.model)); got != c.want {
				t.Errorf("PUT body =\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

// Verify and configure carry the App and the key's secret, and nothing that P0 fixed
// when it created the item.
func TestRepositoryAccessStepJson(t *testing.T) {
	cases := map[string]struct {
		model *repositoryAccessModel
		want  string
	}{
		"aws": {model: awsModel(), want: `{"appId":"123456","privateKeySecretName":"github/my-github-org/private-key"}`},
		"gcp": {model: gcpModel(), want: `{"appId":"234567","privateKeySecretName":"github-private-key"}`},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := marshal(t, (&RepositoryAccess{}).toJson(c.model)); got != c.want {
				t.Errorf("verify and configure body =\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

// Reading back what P0 stores gives the model that was sent, plus the fields P0 fills
// in.
func TestRepositoryAccessFromJson(t *testing.T) {
	cases := map[string]struct {
		model    *repositoryAccessModel
		response string
		// The fields P0 fills in.
		state      string
		serviceUri *string
	}{
		"aws": {
			model: awsModel(),
			response: `{"appId":"123456",` +
				`"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-east-1"},` +
				`"privateKeySecretName":"github/my-github-org/private-key",` +
				`"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-github-repositories-connector","connectorRegion":"us-east-1"},` +
				`"state":"installed"}`,
			state: "installed",
		},
		"gcp": {
			model: gcpModel(),
			response: `{"appId":"234567",` +
				`"vault":{"type":"gcp-sm","install":"my-project-id"},` +
				`"privateKeySecretName":"github-private-key",` +
				`"hosting":{"type":"gcp","projectId":"my-project-id","connectorName":"p0-github-repositories-connector","connectorRegion":"us-central1",` +
				`"connectorServiceUri":"https://p0-github-repositories-connector-abc123-uc.a.run.app"},` +
				`"state":"installed"}`,
			state:      "installed",
			serviceUri: strPtr("https://p0-github-repositories-connector-abc123-uc.a.run.app"),
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var item repositoryAccessJson
			if err := json.Unmarshal([]byte(c.response), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			var diags diag.Diagnostics
			got, ok := (&RepositoryAccess{}).fromJson(context.Background(), &diags, "my-github-org", &item).(*repositoryAccessModel)
			if !ok || diags.HasError() {
				t.Fatalf("fromJson failed: %v", diags)
			}

			want := c.model
			want.State = types.StringValue(c.state)
			want.Hosting.ConnectorServiceUri = types.StringPointerValue(c.serviceUri)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestRepositoryAccessFromJsonRequiresVaultAndHosting(t *testing.T) {
	for name, item := range map[string]repositoryAccessJson{
		"no vault":   {Hosting: awsModel().Hosting.ToJson()},
		"no hosting": {Vault: awsModel().Vault.toJson()},
	} {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := (&RepositoryAccess{}).fromJson(context.Background(), &diags, "my-github-org", &item)
			if got != nil || !diags.HasError() {
				t.Errorf("fromJson = %+v, %v; want nil and an error", got, diags)
			}
		})
	}
}

func repositoryAccessSchema(t *testing.T) schema.Schema {
	t.Helper()
	ctx := context.Background()
	resp := &resource.SchemaResponse{}
	NewRepositoryAccess().Schema(ctx, resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestSchemaIsValid(t *testing.T) {
	if diags := repositoryAccessSchema(t).ValidateImplementation(context.Background()); diags.HasError() {
		t.Errorf("ValidateImplementation: %v", diags)
	}
}

// P0 fixes the vault and the hosting when it creates the item, so changing either
// replaces the installation. The App and the key's secret change in place.
func TestSchemaReplacement(t *testing.T) {
	ctx := context.Background()
	attributes := repositoryAccessSchema(t).Attributes
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
		{name: "org", attribute: attributes["org"], want: true},
		{name: "app_id", attribute: attributes["app_id"], want: false},
		{name: "private_key_secret_name", attribute: attributes["private_key_secret_name"], want: false},
		{name: "vault.type", attribute: nested("vault")["type"], want: true},
		{name: "vault.account_id", attribute: nested("vault")["account_id"], want: true},
		{name: "vault.secrets_region", attribute: nested("vault")["secrets_region"], want: true},
		{name: "vault.project_id", attribute: nested("vault")["project_id"], want: true},
		{name: "hosting.type", attribute: nested("hosting")["type"], want: true},
		{name: "hosting.account_id", attribute: nested("hosting")["account_id"], want: true},
		{name: "hosting.project_id", attribute: nested("hosting")["project_id"], want: true},
		{name: "hosting.connector_name", attribute: nested("hosting")["connector_name"], want: true},
		{name: "hosting.connector_region", attribute: nested("hosting")["connector_region"], want: true},
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
	awsVaultFields   = map[string]tftypes.Value{"type": str("aws-sm"), "account_id": str("123456789012"), "secrets_region": str("us-east-1")}
	gcpVaultFields   = map[string]tftypes.Value{"type": str("gcp-sm"), "project_id": str("my-project-id")}
	awsHostingFields = map[string]tftypes.Value{"type": str("aws"), "account_id": str("123456789012"), "connector_name": str("p0-connector"), "connector_region": str("us-east-1")}
	gcpHostingFields = map[string]tftypes.Value{"type": str("gcp"), "project_id": str("my-project-id"), "connector_name": str("p0-connector"), "connector_region": str("us-central1")}

	awsVault   = objectOf(awsVaultFields)
	gcpVault   = objectOf(gcpVaultFields)
	awsHosting = objectOf(awsHostingFields)
	gcpHosting = objectOf(gcpHostingFields)
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

// Runs the resource's ValidateConfig against a configuration with the given vault and
// hosting. overrides replaces the root string attributes, which are otherwise valid
// for either cloud. The attribute and object validators don't run here, because
// Terraform runs those separately.
func validateConfig(t *testing.T, vault, hosting func(tftypes.Object) tftypes.Value, overrides map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	r := &RepositoryAccess{}
	repositorySchema := repositoryAccessSchema(t)

	objectType, ok := repositorySchema.Type().TerraformType(ctx).(tftypes.Object)
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
		"org":                     str("my-github-org"),
		"app_id":                  str("123456"),
		"private_key_secret_name": str("private-key"),
		"vault":                   vault(nestedType("vault")),
		"hosting":                 hosting(nestedType("hosting")),
	}
	for name, value := range overrides {
		fields[name] = value
	}

	resp := &resource.ValidateConfigResponse{}
	config := tfsdk.Config{Schema: repositorySchema, Raw: objectOf(fields)(objectType)}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	return resp.Diagnostics
}

// Terraform calls ValidateConfig while planning, before the provider calls P0, so these
// are the plan-time checks. Each skips a value Terraform doesn't know yet.
func TestRepositoryAccessValidateConfig(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"
	const arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:github-key"
	const resourceName = "projects/my-project-id/secrets/github-key"
	unknownType := objectOf(map[string]tftypes.Value{"type": unknownString})
	secret := func(value tftypes.Value) map[string]tftypes.Value {
		return map[string]tftypes.Value{"private_key_secret_name": value}
	}

	cases := []struct {
		name      string
		vault     func(tftypes.Object) tftypes.Value
		hosting   func(tftypes.Object) tftypes.Value
		overrides map[string]tftypes.Value
		// The summaries of the errors expected, in any order.
		want []string
	}{
		{name: "aws", vault: awsVault, hosting: awsHosting},
		{name: "gcp", vault: gcpVault, hosting: gcpHosting},
		{name: "aws vault, gcp hosting", vault: awsVault, hosting: gcpHosting, want: []string{"Vault and connector in different clouds"}},
		{name: "gcp vault, aws hosting", vault: gcpVault, hosting: awsHosting, want: []string{"Vault and connector in different clouds"}},
		{name: "unknown vault type", vault: unknownType, hosting: awsHosting},
		{name: "unknown hosting type", vault: gcpVault, hosting: unknownType},
		{name: "unknown vault", vault: unknownObject, hosting: awsHosting},
		{name: "unknown hosting", vault: gcpVault, hosting: unknownObject},
		{name: "unknown vault and hosting", vault: unknownObject, hosting: unknownObject},
		{
			name:    "hosting fields checked as for p0_app",
			vault:   awsVault,
			hosting: objectOf(map[string]tftypes.Value{"type": str("aws"), "project_id": str("my-project-id"), "connector_name": str("p0-connector"), "connector_region": str("us-east-1")}),
			want:    []string{"Missing AWS account ID", "Unexpected Google Cloud project"},
		},
		{name: "aws secret arn", vault: awsVault, hosting: awsHosting, overrides: secret(str(arn))},
		{name: "gcp secret resource name", vault: gcpVault, hosting: gcpHosting, overrides: secret(str(resourceName))},
		{name: "aws secret arn in a gcp vault", vault: gcpVault, hosting: gcpHosting, overrides: secret(str(arn)), want: []string{"Invalid private key secret name"}},
		{name: "pasted key", vault: awsVault, hosting: awsHosting, overrides: secret(str(pem)), want: []string{"Invalid private key secret name"}},
		{name: "unknown secret name", vault: awsVault, hosting: awsHosting, overrides: secret(unknownString)},
		{name: "either vault's form with an unknown vault", vault: unknownObject, hosting: gcpHosting, overrides: secret(str(resourceName))},
		{name: "pasted key with an unknown vault", vault: unknownObject, hosting: unknownObject, overrides: secret(str(pem)), want: []string{"Invalid private key secret name"}},
		{name: "aws secret arn with an unknown hosting", vault: gcpVault, hosting: unknownObject, overrides: secret(str(arn)), want: []string{"Invalid private key secret name"}},
		{name: "secret name with a trailing newline", vault: awsVault, hosting: awsHosting, overrides: secret(str("private-key\n")), want: []string{"Whitespace around a value"}},
		{name: "app id with a leading space", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": str(" 123456")}, want: []string{"Whitespace around a value"}},
		{name: "app id with a trailing newline", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": str("123456\n")}, want: []string{"Whitespace around a value"}},
		{name: "app id that isn't a number", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": str("my-app")}, want: []string{"Invalid GitHub App ID"}},
		{name: "app id with an inner space", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": str("123 456")}, want: []string{"Invalid GitHub App ID"}},
		{name: "empty app id", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": str("")}, want: []string{"Invalid GitHub App ID"}},
		{name: "unknown app id", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"app_id": unknownString}},
		{name: "org with a leading hyphen", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"org": str("-acme")}, want: []string{"Invalid GitHub organization login"}},
		// P0 checks the item's ID as it is.
		{name: "org with a trailing space", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"org": str("acme ")}, want: []string{"Invalid GitHub organization login"}},
		{name: "unknown org", vault: awsVault, hosting: awsHosting, overrides: map[string]tftypes.Value{"org": unknownString}},
		{name: "invalid connector name", vault: awsVault, hosting: with(awsHostingFields, "connector_name", str("P0_Connector")), want: []string{"Invalid connector name"}},
		{name: "unknown connector name", vault: awsVault, hosting: with(awsHostingFields, "connector_name", unknownString)},
		{name: "govcloud connector", vault: awsVault, hosting: with(awsHostingFields, "connector_region", str("us-gov-west-1")), want: []string{"Unsupported AWS region for the connector"}},
		{name: "china secret", vault: with(awsVaultFields, "secrets_region", str("cn-north-1")), hosting: awsHosting, want: []string{"Unsupported AWS region for the private key's secret"}},
		{name: "unknown secrets region", vault: with(awsVaultFields, "secrets_region", unknownString), hosting: awsHosting},
		{name: "invalid vault project", vault: with(gcpVaultFields, "project_id", str("My_Project")), hosting: gcpHosting, want: []string{"Invalid Google Cloud project ID for the private key's secret"}},
		{name: "connector name with whitespace", vault: gcpVault, hosting: with(gcpHostingFields, "connector_name", str("p0-connector ")), want: []string{"Whitespace around a value"}},
		// Each block is checked against its own type's rules, whatever the other's cloud.
		{name: "aws vault with a gcp project id, gcp hosting", vault: awsVault, hosting: with(gcpHostingFields, "project_id", str("Bad")), want: []string{"Vault and connector in different clouds", "Invalid Google Cloud project ID for the connector"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := []string{}
			for _, d := range validateConfig(t, c.vault, c.hosting, c.overrides).Errors() {
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

// A vault and a connector in different clouds are rejected in P0's own words.
func TestRepositoryAccessValidateConfigMixedCloudsWording(t *testing.T) {
	errs := validateConfig(t, awsVault, gcpHosting, nil).Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %v; want one", errs)
	}
	const want = "The vault and the connector need to be in the same cloud. Pick AWS for both or GCP for both."
	if !strings.Contains(errs[0].Detail(), want) {
		t.Errorf("detail = %q; want it to contain %q", errs[0].Detail(), want)
	}
}

// A refused secret name gets P0's message, and isn't repeated back: it may be a key.
func TestRepositoryAccessValidateConfigSecretNameWording(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"
	diags := validateConfig(t, awsVault, awsHosting, map[string]tftypes.Value{"private_key_secret_name": str(pem)})
	errs := diags.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %v; want one", errs)
	}
	if errs[0].Detail() != InvalidSecretName {
		t.Errorf("detail = %q; want %q", errs[0].Detail(), InvalidSecretName)
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok || !withPath.Path().Equal(path.Root("private_key_secret_name")) {
		t.Errorf("error %v isn't on private_key_secret_name", errs[0])
	}
	if strings.Contains(errs[0].Summary()+errs[0].Detail(), "MIIE") {
		t.Errorf("error %v repeats the key", errs[0])
	}
}

// The wording names the organization and shows P0's message, without presuming that
// the connector is at fault.
func TestDescribeCheckError(t *testing.T) {
	const message = "422 Unprocessable Entity: GitHub App 123456 isn't installed on my-github-org. An owner of my-github-org needs to install it, then retry."
	summary, detail := describeCheckError("my-github-org", errors.New(message))
	if summary != "GitHub Repositories install check failed" {
		t.Errorf("summary = %q", summary)
	}
	if want := "P0 rejected the install check for the GitHub organization \"my-github-org\":\n\n" + message; detail != want {
		t.Errorf("detail =\n%s\nwant\n%s", detail, want)
	}
}

func strPtr(s string) *string {
	return &s
}

// The URL that the fake P0 below looks a Cloud Run connector up at.
const connectorServiceUri = "https://p0-github-repositories-connector-abc123-uc.a.run.app"

// model at state, with the URL P0 looked a Cloud Run connector up at.
func at(model *repositoryAccessModel, state types.String, serviceUri types.String) *repositoryAccessModel {
	model.State = state
	model.Hosting.ConnectorServiceUri = serviceUri
	return model
}

// model as the resource's state or plan holds it, or null for a nil model.
func rawOf(t *testing.T, model *repositoryAccessModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	repositorySchema := repositoryAccessSchema(t)
	state := tfsdk.State{Schema: repositorySchema, Raw: tftypes.NewValue(repositorySchema.Type().TerraformType(ctx), nil)}
	if model == nil {
		return state.Raw
	}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("Set: %v", diags)
	}
	return state.Raw
}

// A fake of P0's verify and configure steps for item, which records each request. A
// step answers with item, with the App and the key's secret that the step was sent,
// at the state that the step moves the item to, and on Cloud Run with the URL that
// P0 looks the connector up at. With failConfigure, configure answers as P0 does when
// it can't reach the connector, and P0 saves nothing then.
type fakeSteps struct {
	item          *repositoryAccessModel
	failConfigure bool

	mu       sync.Mutex
	requests []string
}

func newFakeSteps(t *testing.T, item *repositoryAccessModel, failConfigure bool) (*fakeSteps, *httptest.Server) {
	t.Helper()
	fake := &fakeSteps{item: item, failConfigure: failConfigure}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

func (f *fakeSteps) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, fmt.Sprintf("%s %s %s", r.Method, r.URL.Path, body))
	f.mu.Unlock()

	step := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	nextState := map[string]string{common.Verify: common.StateConfigure, common.Config: common.StateInstalled}[step]
	var sent repositoryAccessConfigureJson
	if r.Method != http.MethodPost || nextState == "" || json.Unmarshal(body, &sent) != nil {
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if step == common.Config && f.failConfigure {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"github-repositories is temporarily unavailable; please try again later"}`))
		return
	}
	item := stageJson(f.item)
	item.AppId = &sent.AppId
	item.PrivateKeySecretName = &sent.PrivateKeySecretName
	if item.Hosting.Type == installapp.GcpHosting {
		item.Hosting.ConnectorServiceUri = strPtr(connectorServiceUri)
	}
	item.State = &nextState
	_ = json.NewEncoder(w).Encode(repositoryAccessApi{Item: item})
}

func (f *fakeSteps) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

// The resource, configured against the fake P0 at server.
func configuredResource(t *testing.T, server *httptest.Server) *RepositoryAccess {
	t.Helper()
	r := &RepositoryAccess{}
	resp := &resource.ConfigureResponse{}
	data := internal.P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: data}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure: %v", resp.Diagnostics)
	}
	return r
}

// Runs Update from prior to planned as the framework does, which starts the response's
// state as the prior state.
func update(t *testing.T, r *RepositoryAccess, prior, planned tftypes.Value) *resource.UpdateResponse {
	t.Helper()
	repositorySchema := repositoryAccessSchema(t)
	req := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: repositorySchema, Raw: planned},
		State: tfsdk.State{Schema: repositorySchema, Raw: prior},
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: repositorySchema, Raw: prior.Copy()}}
	r.Update(context.Background(), req, resp)
	return resp
}

// Runs ModifyPlan on the framework's plan for prior as the framework does, which starts
// the response's plan as its own.
func modifyPlan(t *testing.T, prior, planned tftypes.Value) *resource.ModifyPlanResponse {
	t.Helper()
	repositorySchema := repositoryAccessSchema(t)
	req := resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: repositorySchema, Raw: planned},
		State: tfsdk.State{Schema: repositorySchema, Raw: prior},
	}
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: repositorySchema, Raw: planned.Copy()}}
	(&RepositoryAccess{}).ModifyPlan(context.Background(), req, resp)
	return resp
}

const itemPath = "/integrations/github-repositories/config/repository-access/my-github-org"

// An update of an item that P0 has verified, at configure or installed, posts the
// configure step alone, with the new App, so P0 checks the install once. A staged item
// is verified first, as on create.
func TestRepositoryAccessUpdateSteps(t *testing.T) {
	const body = `{"appId":"777777","privateKeySecretName":"github/my-github-org/private-key"}`
	verify := "POST " + itemPath + "/verify " + body
	configure := "POST " + itemPath + "/configure " + body

	cases := []struct {
		prior string
		want  []string
	}{
		{prior: common.StateInstalled, want: []string{configure}},
		{prior: common.StateConfigure, want: []string{configure}},
		{prior: common.StateStage, want: []string{verify, configure}},
	}

	for _, c := range cases {
		t.Run(c.prior, func(t *testing.T) {
			fake, server := newFakeSteps(t, awsModel(), false)
			prior := rawOf(t, at(awsModel(), types.StringValue(c.prior), types.StringNull()))
			planned := at(awsModel(), types.StringUnknown(), types.StringUnknown())
			planned.AppId = types.StringValue("777777")

			resp := update(t, configuredResource(t, server), prior, rawOf(t, planned))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			if got := fake.Requests(); !reflect.DeepEqual(got, c.want) {
				t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
			want := at(awsModel(), types.StringValue(common.StateInstalled), types.StringNull())
			want.AppId = types.StringValue("777777")
			if !resp.State.Raw.Equal(rawOf(t, want)) {
				t.Errorf("state = %v; want the installed item with the new App", resp.State.Raw)
			}
		})
	}
}

// When the check fails on an installed item's update, the apply fails and nothing
// changes: P0 saves nothing when configure fails, and the state keeps the installed
// item and its App, so the next plan shows the update again.
func TestRepositoryAccessUpdateFailedCheck(t *testing.T) {
	fake, server := newFakeSteps(t, awsModel(), true)
	prior := rawOf(t, at(awsModel(), types.StringValue(common.StateInstalled), types.StringNull()))
	planned := at(awsModel(), types.StringUnknown(), types.StringUnknown())
	planned.AppId = types.StringValue("777777")

	resp := update(t, configuredResource(t, server), prior, rawOf(t, planned))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "GitHub Repositories install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	want := []string{"POST " + itemPath + `/configure {"appId":"777777","privateKeySecretName":"github/my-github-org/private-key"}`}
	if got := fake.Requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %v; want %v", got, want)
	}
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want the installed item kept", resp.State.Raw)
	}
}

// An item that P0 hasn't installed plans an update even when its configuration hasn't
// changed, so that the next apply finishes the install, and the attributes that only
// P0 sets are unknown in it, as in any update. Every other plan is the framework's, so
// an installed item plans no difference.
func TestRepositoryAccessModifyPlan(t *testing.T) {
	installed := types.StringValue(common.StateInstalled)
	configured := types.StringValue(common.StateConfigure)
	staged := types.StringValue(common.StateStage)
	unknown := types.StringUnknown()
	none := types.StringNull()
	uri := types.StringValue(connectorServiceUri)

	gcpAt := func(state, serviceUri types.String) tftypes.Value {
		return rawOf(t, at(gcpModel(), state, serviceUri))
	}
	awsAt := func(state, serviceUri types.String) tftypes.Value {
		return rawOf(t, at(awsModel(), state, serviceUri))
	}
	// The framework's plan for a new App: the attributes that only P0 sets are unknown.
	newApp := func() tftypes.Value {
		model := at(gcpModel(), unknown, unknown)
		model.AppId = types.StringValue("777777")
		return rawOf(t, model)
	}

	cases := []struct {
		name  string
		prior tftypes.Value // The refreshed state, null on create.
		plan  tftypes.Value // The framework's plan, null on destroy.
		want  tftypes.Value
	}{
		{name: "installed", prior: gcpAt(installed, uri), plan: gcpAt(installed, uri), want: gcpAt(installed, uri)},
		{name: "installed aws", prior: awsAt(installed, none), plan: awsAt(installed, none), want: awsAt(installed, none)},
		{name: "installed, with a new App", prior: gcpAt(installed, uri), plan: newApp(), want: newApp()},
		{name: "configure", prior: gcpAt(configured, uri), plan: gcpAt(configured, uri), want: gcpAt(unknown, unknown)},
		{name: "configure, with a new App", prior: gcpAt(configured, uri), plan: newApp(), want: newApp()},
		// P0 drops the connector's URL when it stages an item, and looks it up again
		// when it verifies it.
		{name: "stage", prior: gcpAt(staged, none), plan: gcpAt(staged, none), want: gcpAt(unknown, unknown)},
		{name: "stage aws", prior: awsAt(staged, none), plan: awsAt(staged, none), want: awsAt(unknown, unknown)},
		{name: "no state", prior: gcpAt(none, uri), plan: gcpAt(none, uri), want: gcpAt(unknown, unknown)},
		{name: "create", prior: rawOf(t, nil), plan: gcpAt(unknown, unknown), want: gcpAt(unknown, unknown)},
		{name: "destroy", prior: gcpAt(staged, none), plan: rawOf(t, nil), want: rawOf(t, nil)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := modifyPlan(t, c.prior, c.plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
			}
			if !resp.Plan.Raw.Equal(c.want) {
				t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, c.want)
			}
		})
	}
}

// A hosting block that Terraform only knows at apply time stays unknown.
func TestRepositoryAccessModifyPlanUnknownHosting(t *testing.T) {
	ctx := context.Background()
	repositorySchema := repositoryAccessSchema(t)
	hostingType, ok := repositorySchema.Attributes["hosting"].GetType().(types.ObjectType)
	if !ok {
		t.Fatalf("hosting is not an object")
	}
	withUnknownHosting := func(state types.String) tftypes.Value {
		plan := tfsdk.Plan{Schema: repositorySchema, Raw: rawOf(t, at(gcpModel(), state, types.StringNull()))}
		if diags := plan.SetAttribute(ctx, path.Root("hosting"), types.ObjectUnknown(hostingType.AttrTypes)); diags.HasError() {
			t.Fatalf("SetAttribute: %v", diags)
		}
		return plan.Raw
	}

	prior := rawOf(t, at(gcpModel(), types.StringValue(common.StateConfigure), types.StringValue(connectorServiceUri)))
	resp := modifyPlan(t, prior, withUnknownHosting(types.StringValue(common.StateConfigure)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
	}
	if want := withUnknownHosting(types.StringUnknown()); !resp.Plan.Raw.Equal(want) {
		t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, want)
	}
}

// An item imported at stage converges in one apply: its plan shows an update, the
// update verifies and configures it, and the plan after that shows no difference.
func TestRepositoryAccessStagedItemConverges(t *testing.T) {
	// P0 drops the connector's URL when it stages an item.
	staged := rawOf(t, at(gcpModel(), types.StringValue(common.StateStage), types.StringNull()))
	plan := modifyPlan(t, staged, staged)
	if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
		t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
	}

	fake, server := newFakeSteps(t, gcpModel(), false)
	resp := update(t, configuredResource(t, server), staged, plan.Plan.Raw)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	const body = `{"appId":"234567","privateKeySecretName":"github-private-key"}`
	want := []string{"POST " + itemPath + "/verify " + body, "POST " + itemPath + "/configure " + body}
	if got := fake.Requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	installed := rawOf(t, at(gcpModel(), types.StringValue(common.StateInstalled), types.StringValue(connectorServiceUri)))
	if !resp.State.Raw.Equal(installed) {
		t.Fatalf("state = %v; want the installed item", resp.State.Raw)
	}
	if replan := modifyPlan(t, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
		t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
	}
}
