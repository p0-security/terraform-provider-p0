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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
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
			SecretsRegion: types.StringValue("us-east-1"),
		},
		PrivateKeySecretName: types.StringValue("github/my-github-org/private-key"),
		Hosting: &hostingModel{
			Type:            types.StringValue(installapp.AwsHosting),
			AccountId:       types.StringValue("123456789012"),
			ConnectorName:   types.StringValue("p0-github-repositories-connector"),
			ConnectorRegion: types.StringValue("us-east-1"),
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
		// The field P0 fills in.
		state string
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
			if !reflect.DeepEqual(got, want) {
				t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestRepositoryAccessFromJsonRequiresVaultAndHosting(t *testing.T) {
	for name, item := range map[string]repositoryAccessJson{
		"no vault":   {Hosting: awsModel().Hosting.toJson()},
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

// This resource supports only AWS for now, so an item in another cloud, such as one
// created in the P0 app, is refused rather than read into the wrong fields.
func TestRepositoryAccessFromJsonRequiresAws(t *testing.T) {
	const awsVaultJson = `"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-east-1"}`
	const awsHostingJson = `"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-connector","connectorRegion":"us-east-1"}`
	const gcpVaultJson = `"vault":{"type":"gcp-sm","install":"my-project-id"}`
	const gcpHostingJson = `"hosting":{"type":"gcp","projectId":"my-project-id","connectorName":"p0-connector","connectorRegion":"us-central1"}`

	for name, response := range map[string]string{
		"gcp":                    `{"appId":"123456",` + gcpVaultJson + `,"privateKeySecretName":"key",` + gcpHostingJson + `}`,
		"gcp vault, aws hosting": `{"appId":"123456",` + gcpVaultJson + `,"privateKeySecretName":"key",` + awsHostingJson + `}`,
		"aws vault, gcp hosting": `{"appId":"123456",` + awsVaultJson + `,"privateKeySecretName":"key",` + gcpHostingJson + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			var item repositoryAccessJson
			if err := json.Unmarshal([]byte(response), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			var diags diag.Diagnostics
			got := (&RepositoryAccess{}).fromJson(context.Background(), &diags, "my-github-org", &item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Unsupported GitHub Repositories install" {
				t.Errorf("fromJson = %+v, %v; want nil and an unsupported install error", got, diags)
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

// P0 fixes the vault, the hosting and the key's secret when it creates the item, so
// changing any of them replaces the installation. The App changes in place.
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
		{name: "private_key_secret_name", attribute: attributes["private_key_secret_name"], want: true},
		{name: "vault.type", attribute: nested("vault")["type"], want: true},
		{name: "vault.account_id", attribute: nested("vault")["account_id"], want: true},
		{name: "vault.secrets_region", attribute: nested("vault")["secrets_region"], want: true},
		{name: "hosting.type", attribute: nested("hosting")["type"], want: true},
		{name: "hosting.account_id", attribute: nested("hosting")["account_id"], want: true},
		{name: "hosting.connector_name", attribute: nested("hosting")["connector_name"], want: true},
		{name: "hosting.connector_region", attribute: nested("hosting")["connector_region"], want: true},
	}

	for _, c := range cases {
		if got := replaces(c.attribute); got != c.want {
			t.Errorf("%s requires replacement = %v; want %v", c.name, got, c.want)
		}
	}
}

// planTestProvider serves only this package's resource, so a test can plan through
// the framework as Terraform does.
type planTestProvider struct{}

func (*planTestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "p0"
}

func (*planTestProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {}

func (*planTestProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (*planTestProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewRepositoryAccess}
}

func (*planTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

// Plans an installed AWS item's change to configured, as Terraform does: the proposed
// state is the configuration with the prior state's computed values.
func planChange(t *testing.T, configured *repositoryAccessModel) *tfprotov6.PlanResourceChangeResponse {
	t.Helper()
	ctx := context.Background()
	objectType := repositoryAccessSchema(t).Type().TerraformType(ctx)
	dynamic := func(model *repositoryAccessModel) *tfprotov6.DynamicValue {
		value, err := tfprotov6.NewDynamicValue(objectType, rawOf(t, model))
		if err != nil {
			t.Fatalf("NewDynamicValue: %v", err)
		}
		return &value
	}
	proposed := *configured
	proposed.State = types.StringValue(common.StateInstalled)

	server := providerserver.NewProtocol6(&planTestProvider{})()
	resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "p0_github_repositories",
		PriorState:       dynamic(at(awsModel(), types.StringValue(common.StateInstalled))),
		ProposedNewState: dynamic(&proposed),
		Config:           dynamic(configured),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("PlanResourceChange: %s: %s", d.Summary, d.Detail)
		}
	}
	return resp
}

// A new secret for the key plans a replacement, since P0 refuses to change it on an
// item. A new App plans an update in place.
func TestRepositoryAccessPlanReplacement(t *testing.T) {
	cases := []struct {
		name   string
		change func(*repositoryAccessModel)
		want   []*tftypes.AttributePath
	}{
		{name: "unchanged", change: func(*repositoryAccessModel) {}},
		{
			name:   "new App",
			change: func(model *repositoryAccessModel) { model.AppId = types.StringValue("777777") },
		},
		{
			name: "new secret",
			change: func(model *repositoryAccessModel) {
				model.PrivateKeySecretName = types.StringValue("github/my-github-org/new-private-key")
			},
			want: []*tftypes.AttributePath{tftypes.NewAttributePath().WithAttributeName("private_key_secret_name")},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			configured := awsModel()
			c.change(configured)
			got := planChange(t, configured).RequiresReplace
			if len(got) != len(c.want) {
				t.Fatalf("requires replacement = %v; want %v", got, c.want)
			}
			for i := range got {
				if !got[i].Equal(c.want[i]) {
					t.Errorf("requires replacement = %v; want %v", got, c.want)
				}
			}
		})
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
	awsHostingFields = map[string]tftypes.Value{"type": str("aws"), "account_id": str("123456789012"), "connector_name": str("p0-connector"), "connector_region": str("us-east-1")}

	awsVault   = objectOf(awsVaultFields)
	awsHosting = objectOf(awsHostingFields)
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
// hosting. overrides replaces the root string attributes, which are otherwise valid.
// The attribute and object validators don't run here, because
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
		{name: "unknown vault type", vault: unknownType, hosting: awsHosting},
		{name: "unknown hosting type", vault: awsVault, hosting: unknownType},
		{name: "unknown vault", vault: unknownObject, hosting: awsHosting},
		{name: "unknown hosting", vault: awsVault, hosting: unknownObject},
		{name: "unknown vault and hosting", vault: unknownObject, hosting: unknownObject},
		{name: "aws secret arn", vault: awsVault, hosting: awsHosting, overrides: secret(str(arn))},
		{name: "pasted key", vault: awsVault, hosting: awsHosting, overrides: secret(str(pem)), want: []string{"Invalid private key secret name"}},
		{name: "unknown secret name", vault: awsVault, hosting: awsHosting, overrides: secret(unknownString)},
		{name: "aws secret arn with an unknown vault", vault: unknownObject, hosting: awsHosting, overrides: secret(str(arn))},
		{name: "pasted key with an unknown vault", vault: unknownObject, hosting: unknownObject, overrides: secret(str(pem)), want: []string{"Invalid private key secret name"}},
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
		{name: "connector name with whitespace", vault: awsVault, hosting: with(awsHostingFields, "connector_name", str("p0-connector ")), want: []string{"Whitespace around a value"}},
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

// model at state.
func at(model *repositoryAccessModel, state types.String) *repositoryAccessModel {
	model.State = state
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
// at the state that the step moves the item to. With failConfigure, configure answers as P0 does when
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
			prior := rawOf(t, at(awsModel(), types.StringValue(c.prior)))
			planned := at(awsModel(), types.StringUnknown())
			planned.AppId = types.StringValue("777777")

			resp := update(t, configuredResource(t, server), prior, rawOf(t, planned))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			if got := fake.Requests(); !reflect.DeepEqual(got, c.want) {
				t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
			want := at(awsModel(), types.StringValue(common.StateInstalled))
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
	prior := rawOf(t, at(awsModel(), types.StringValue(common.StateInstalled)))
	planned := at(awsModel(), types.StringUnknown())
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
// changed, so that the next apply finishes the install, and its state, which only P0
// sets, is unknown in it, as in any update. Every other plan is the framework's, so an
// installed item plans no difference.
func TestRepositoryAccessModifyPlan(t *testing.T) {
	installed := types.StringValue(common.StateInstalled)
	configured := types.StringValue(common.StateConfigure)
	staged := types.StringValue(common.StateStage)
	unknown := types.StringUnknown()
	none := types.StringNull()

	awsAt := func(state types.String) tftypes.Value {
		return rawOf(t, at(awsModel(), state))
	}
	// The framework's plan for a new App: the state, which only P0 sets, is unknown.
	newApp := func() tftypes.Value {
		model := at(awsModel(), unknown)
		model.AppId = types.StringValue("777777")
		return rawOf(t, model)
	}

	cases := []struct {
		name  string
		prior tftypes.Value // The refreshed state, null on create.
		plan  tftypes.Value // The framework's plan, null on destroy.
		want  tftypes.Value
	}{
		{name: "installed", prior: awsAt(installed), plan: awsAt(installed), want: awsAt(installed)},
		{name: "installed, with a new App", prior: awsAt(installed), plan: newApp(), want: newApp()},
		{name: "configure", prior: awsAt(configured), plan: awsAt(configured), want: awsAt(unknown)},
		{name: "configure, with a new App", prior: awsAt(configured), plan: newApp(), want: newApp()},
		{name: "stage", prior: awsAt(staged), plan: awsAt(staged), want: awsAt(unknown)},
		{name: "no state", prior: awsAt(none), plan: awsAt(none), want: awsAt(unknown)},
		{name: "create", prior: rawOf(t, nil), plan: awsAt(unknown), want: awsAt(unknown)},
		{name: "destroy", prior: awsAt(staged), plan: rawOf(t, nil), want: rawOf(t, nil)},
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
		plan := tfsdk.Plan{Schema: repositorySchema, Raw: rawOf(t, at(awsModel(), state))}
		if diags := plan.SetAttribute(ctx, path.Root("hosting"), types.ObjectUnknown(hostingType.AttrTypes)); diags.HasError() {
			t.Fatalf("SetAttribute: %v", diags)
		}
		return plan.Raw
	}

	prior := rawOf(t, at(awsModel(), types.StringValue(common.StateConfigure)))
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
	staged := rawOf(t, at(awsModel(), types.StringValue(common.StateStage)))
	plan := modifyPlan(t, staged, staged)
	if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
		t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
	}

	fake, server := newFakeSteps(t, awsModel(), false)
	resp := update(t, configuredResource(t, server), staged, plan.Plan.Raw)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	const body = `{"appId":"123456","privateKeySecretName":"github/my-github-org/private-key"}`
	want := []string{"POST " + itemPath + "/verify " + body, "POST " + itemPath + "/configure " + body}
	if got := fake.Requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	installed := rawOf(t, at(awsModel(), types.StringValue(common.StateInstalled)))
	if !resp.State.Raw.Equal(installed) {
		t.Fatalf("state = %v; want the installed item", resp.State.Raw)
	}
	if replan := modifyPlan(t, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
		t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
	}
}
