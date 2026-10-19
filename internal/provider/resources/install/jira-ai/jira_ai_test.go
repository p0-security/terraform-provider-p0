package installjiraai

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// schemaOf, decodeNullState, marshal, accepts, nestedAttributes and requiresReplace
// are copies of the onepassword package's test helpers (onepassword/common_test.go).

func schemaOf(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// Decodes a state with every schema attribute null into the model, as the framework
// does on import.
func decodeNullState(t *testing.T, r resource.Resource, model any) {
	t.Helper()
	ctx := context.Background()
	s := schemaOf(t, r)
	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	values := map[string]tftypes.Value{}
	for name, attrType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(objectType, values)}
	if diags := state.Get(ctx, model); diags.HasError() {
		t.Fatalf("decode: %v", diags)
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

// Whether attribute's validators accept value.
func accepts(t *testing.T, attribute schema.Attribute, value string) bool {
	t.Helper()
	stringAttribute, ok := attribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("%v is not a string attribute", attribute)
	}
	resp := &validator.StringResponse{}
	for _, v := range stringAttribute.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("value"), ConfigValue: types.StringValue(value)}, resp)
	}
	return !resp.Diagnostics.HasError()
}

func nestedAttributes(t *testing.T, s schema.Schema, name string) map[string]schema.Attribute {
	t.Helper()
	attribute, ok := s.Attributes[name].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("%s is not a nested attribute", name)
	}
	return attribute.Attributes
}

func requiresReplace(attribute schema.StringAttribute) bool {
	ctx := context.Background()
	for _, modifier := range attribute.PlanModifiers {
		if modifier.Description(ctx) == stringplanmodifier.RequiresReplace().Description(ctx) {
			return true
		}
	}
	return false
}

func resources() map[string]resource.Resource {
	return map[string]resource.Resource{
		"p0_jira_ai_connector_staged": NewJiraAiConnectorStaged(),
		"p0_jira_ai_connector":        NewJiraAiConnector(),
		"p0_jira_ai_site":             NewJiraAiSite(),
		"p0_jira_ai_pool_account":     NewJiraAiPoolAccount(),
	}
}

// Each resource's schema is one the framework accepts, and its model has exactly the
// schema's attributes: a field the schema lacks, or an attribute the model lacks, fails
// every plan and apply with a conversion error that neither `go build` nor `go vet`
// catches.
func TestSchemasAndModels(t *testing.T) {
	models := map[string]any{
		"p0_jira_ai_connector_staged": &connectorModel{},
		"p0_jira_ai_connector":        &connectorModel{},
		"p0_jira_ai_site":             &siteModel{},
		"p0_jira_ai_pool_account":     &poolAccountModel{},
	}

	for name, r := range resources() {
		t.Run(name, func(t *testing.T) {
			if diags := schemaOf(t, r).ValidateImplementation(context.Background()); diags.HasError() {
				t.Errorf("ValidateImplementation: %v", diags)
			}
			decodeNullState(t, r, models[name])
		})
	}
}

func TestResourceNames(t *testing.T) {
	for want, r := range resources() {
		resp := &resource.MetadataResponse{}
		r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "p0"}, resp)
		if resp.TypeName != want {
			t.Errorf("type name = %q; want %q", resp.TypeName, want)
		}
	}
}

// Every field is `step: "new"` in the app's install schema, so P0 refuses a change to
// any of them once the item exists. Each attribute a configuration sets requires
// replacement, and each that P0 sets is read-only. The integration is in beta, and has
// no feature flag to advise about.
func TestSchemas(t *testing.T) {
	for name, r := range resources() {
		t.Run(name, func(t *testing.T) {
			s := schemaOf(t, r)
			if !strings.Contains(s.MarkdownDescription, betaNote) {
				t.Errorf("description doesn't say the integration is in beta")
			}
			for _, word := range []string{"feature flag", "isn't enabled", "preview"} {
				if strings.Contains(s.MarkdownDescription, word) {
					t.Errorf("description mentions %q", word)
				}
			}

			check := func(attributes map[string]schema.Attribute, prefix string) {
				for attrName, attribute := range attributes {
					stringAttribute, ok := attribute.(schema.StringAttribute)
					if !ok {
						continue
					}
					switch {
					case stringAttribute.Required && !requiresReplace(stringAttribute):
						t.Errorf("%s%s doesn't require replacement", prefix, attrName)
					case stringAttribute.Computed && (stringAttribute.Optional || stringAttribute.Required):
						t.Errorf("%s%s can be set, but P0 sets it", prefix, attrName)
					}
				}
			}
			check(s.Attributes, "")
			for attrName, attribute := range s.Attributes {
				if nested, ok := attribute.(schema.SingleNestedAttribute); ok {
					check(nested.Attributes, attrName+".")
				}
			}
		})
	}
}

// P0's rules for the items' IDs (item-id.ts in the app).
func TestIdRules(t *testing.T) {
	cases := []struct {
		id         string
		item, site bool
	}{
		{"prod", true, true},
		{"0", true, true},
		{"acme-eu", true, true},
		{strings.Repeat("a", 48), true, true},
		{strings.Repeat("a", 49), true, false},
		{strings.Repeat("a", 63), true, false},
		{strings.Repeat("a", 64), false, false},
		{"", false, false},
		{"-prod", false, false},
		{"prod-", false, false},
		{"Prod", false, false},
		{"acme_eu", false, false},
		{"acme/../x", false, false},
		{" prod", false, false},
	}
	for _, c := range cases {
		if got := isItemId(c.id); got != c.item {
			t.Errorf("isItemId(%q) = %v; want %v", c.id, got, c.item)
		}
		if got := isSiteId(c.id); got != c.site {
			t.Errorf("isSiteId(%q) = %v; want %v", c.id, got, c.site)
		}
	}

	site := schemaOf(t, NewJiraAiSite())
	pool := schemaOf(t, NewJiraAiPoolAccount())
	for _, c := range cases {
		if c.id == "" {
			continue
		}
		if got := accepts(t, site.Attributes["id"], c.id); got != c.site {
			t.Errorf("site id accepts %q = %v; want %v", c.id, got, c.site)
		}
		if got := accepts(t, pool.Attributes["site"], c.id); got != c.site {
			t.Errorf("agent account site accepts %q = %v; want %v", c.id, got, c.site)
		}
		if got := accepts(t, pool.Attributes["id"], c.id); got != c.item {
			t.Errorf("agent account id accepts %q = %v; want %v", c.id, got, c.item)
		}
	}
}

// P0's rules for the connector's hosting (connector-hosting.ts and gcp-hosting.ts in
// the app).
func TestHostingValidators(t *testing.T) {
	hosting := nestedAttributes(t, schemaOf(t, NewJiraAiConnectorStaged()), "hosting")
	cases := map[string]map[string]bool{
		"type":             {"gcp": true, "aws": false},
		"project_id":       {"acme-prod": true, "123456789012": false, "Acme-Prod": false, "acme-prod ": false, "acme_prod": false},
		"connector_name":   {"p0-jira-prod": true, "p0jira": true, "p0-j": false, "p0-jira-connector-for-production": false, "p0-jira-": false},
		"connector_region": {"us-central1": true, "europe-west1": true, "us-central1-a": false, "us-east-1": false, "us-central1 ": false},
	}
	for field, values := range cases {
		for value, want := range values {
			if got := accepts(t, hosting[field], value); got != want {
				t.Errorf("hosting.%s accepts %q = %v; want %v", field, value, got, want)
			}
		}
	}
}

// P0's rules for a site's and an agent account's fields (site-url.ts, secret-name.ts,
// site.ts and pool-account.ts in the app), and whitespace that P0 would trim.
func TestItemValidators(t *testing.T) {
	site := schemaOf(t, NewJiraAiSite())
	pool := schemaOf(t, NewJiraAiPoolAccount())
	vault := nestedAttributes(t, site, "vault")
	cases := []struct {
		attribute schema.Attribute
		name      string
		values    map[string]bool
	}{
		{site.Attributes["site_url"], "site_url", map[string]bool{
			"https://acme.atlassian.net":       true,
			"https://acme.atlassian.net/":      true,
			"https://Acme.Atlassian.net":       true,
			"http://acme.atlassian.net":        false,
			"https://acme.atlassian.net/jira":  false,
			"https://acme.atlassian.net:443":   false,
			"https://user@acme.atlassian.net":  false,
			"https://acme.atlassian.net.evil":  false,
			"https://jira.acme.com":            false,
			" https://acme.atlassian.net":      false,
			"https://a.b.atlassian.net":        false,
			"https://-acme.atlassian.net":      false,
			"https://acme-eu.atlassian.net":    true,
			"https://acme.jira.atlassian.net/": false,
		}},
		{site.Attributes["admin_secret_name"], "admin_secret_name", map[string]bool{
			"jira-acme-admin": true, "jira_acme_admin": true, strings.Repeat("a", 255): true, strings.Repeat("a", 256): false,
			"projects/p/secrets/s": false, "jira.acme": false, "jira acme": false,
		}},
		{site.Attributes["admin_client_id"], "admin_client_id", map[string]bool{"abc": true, " abc": false, "abc ": false, " ": false}},
		{vault["type"], "vault.type", map[string]bool{"gcp-sm": true, "aws-sm": false}},
		{vault["project_id"], "vault.project_id", map[string]bool{"acme-secrets": true, "123456789012": false}},
		{pool.Attributes["email"], "email", map[string]bool{"agent@serviceaccount.atlassian.com": true, " agent@x.com": false, "": false}},
	}
	for _, c := range cases {
		for value, want := range c.values {
			if got := accepts(t, c.attribute, value); got != want {
				t.Errorf("%s accepts %q = %v; want %v", c.name, value, got, want)
			}
		}
	}
}

func gcpHosting() *hostingModel {
	return &hostingModel{
		Type:                    types.StringValue("gcp"),
		ProjectId:               types.StringValue("acme-prod"),
		ConnectorName:           types.StringValue("p0-jira-prod"),
		ConnectorRegion:         types.StringValue("us-central1"),
		ConnectorServiceAccount: types.StringValue("p0-jira-prod@acme-prod.iam.gserviceaccount.com"),
		ConnectorServiceUri:     types.StringValue("https://p0-jira-prod-abc123-uc.a.run.app"),
	}
}

// Every request carries where the connector runs, and leaves out what P0 sets.
func TestConnectorToJson(t *testing.T) {
	got := marshal(t, connectorToJson(&connectorModel{Id: types.StringValue("prod"), Hosting: gcpHosting(), State: types.StringValue("installed")}))
	want := `{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-prod","connectorRegion":"us-central1"}}`
	if got != want {
		t.Errorf("body = %s; want %s", got, want)
	}
}

func TestConnectorFromJson(t *testing.T) {
	read := func(body string, missing string) (any, diag.Diagnostics) {
		var api connectorApi
		if err := json.Unmarshal([]byte(body), &api); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var diags diag.Diagnostics
		return connectorFromJson(missing)(context.Background(), &diags, "prod", connectorItemJson(&api)), diags
	}

	got, diags := read(`{"item":{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-prod","connectorRegion":"us-central1",`+
		`"connectorServiceAccount":"p0-jira-prod@acme-prod.iam.gserviceaccount.com","connectorServiceUri":"https://p0-jira-prod-abc123-uc.a.run.app"},"state":"installed"}}`,
		missingServiceAccountOnWrite)
	want := &connectorModel{Id: types.StringValue("prod"), Hosting: gcpHosting(), State: types.StringValue("installed")}
	if diags.HasError() || !reflect.DeepEqual(got, want) {
		t.Errorf("fromJson = %+v, %v; want %+v", got, diags, want)
	}

	for name, c := range map[string]struct{ body, missing, want string }{
		"no hosting": {`{"item":{"state":"stage"}}`, missingServiceAccountOnWrite, "carries no hosting configuration"},
		"no service account on write": {
			`{"item":{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-prod","connectorRegion":"us-central1"},"state":"stage"}}`,
			missingServiceAccountOnWrite, "Contact support@p0.dev.",
		},
		"blank service account on read": {
			`{"item":{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-jira-prod","connectorRegion":"us-central1","connectorServiceAccount":""},"state":"installed"}}`,
			missingServiceAccountOnRead, "-refresh=false",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, diags := read(c.body, c.missing)
			if got != nil || !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), c.want) {
				t.Errorf("fromJson = %v, %v; want an error that says %q", got, diags, c.want)
			}
		})
	}
}

// A response with no item isn't read as one: a nil item in an any isn't nil.
func TestItemJsonRequiresItem(t *testing.T) {
	for name, got := range map[string]any{
		"connector":     connectorItemJson(&connectorApi{}),
		"site":          siteItemJson(&siteApi{}),
		"agent account": poolAccountItemJson(&poolAccountApi{}),
	} {
		if got != nil {
			t.Errorf("%s item = %#v; want nil", name, got)
		}
	}
}

func site() *siteModel {
	return &siteModel{
		Id:              types.StringValue("acme"),
		Connector:       types.StringValue("prod"),
		SiteUrl:         types.StringValue("https://acme.atlassian.net"),
		CloudId:         types.StringValue("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"),
		Vault:           &vaultModel{Type: types.StringValue("gcp-sm"), ProjectId: types.StringValue("acme-secrets")},
		AdminClientId:   types.StringValue("admin-client"),
		AdminSecretName: types.StringValue("jira-acme-admin"),
		State:           types.StringValue("installed"),
	}
}

// Every request carries the whole site but the cloud ID, which P0 looks up, and the
// vault names its installation as P0 does. Reading the stored item gives the model
// back.
func TestSiteJson(t *testing.T) {
	body := marshal(t, siteToJson(site()))
	want := `{"connector":"prod","siteUrl":"https://acme.atlassian.net","vault":{"type":"gcp-sm","install":"acme-secrets"},` +
		`"adminClientId":"admin-client","adminSecretName":"jira-acme-admin"}`
	if body != want {
		t.Errorf("body = %s; want %s", body, want)
	}

	var api siteApi
	stored := `{"item":` + strings.TrimSuffix(want, "}") + `,"cloudId":"0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0","state":"installed"}}`
	if err := json.Unmarshal([]byte(stored), &api); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var diags diag.Diagnostics
	if got := siteFromJson(context.Background(), &diags, "acme", siteItemJson(&api)); diags.HasError() || !reflect.DeepEqual(got, site()) {
		t.Errorf("fromJson = %+v, %v; want %+v", got, diags, site())
	}
}

// A site without a vault, or with one this provider doesn't support, can't be read
// into the model, and says what to do.
func TestSiteFromJsonVaults(t *testing.T) {
	for name, c := range map[string]struct{ vault, want string }{
		"no vault":  {``, "without its vault"},
		"AWS vault": {`,"vault":{"type":"aws-sm","install":"123456789012","secretsRegion":"us-west-2"}`, `has a "aws-sm" vault`},
	} {
		t.Run(name, func(t *testing.T) {
			var api siteApi
			if err := json.Unmarshal([]byte(`{"item":{"connector":"prod"`+c.vault+`,"state":"installed"}}`), &api); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			var diags diag.Diagnostics
			got := siteFromJson(context.Background(), &diags, "acme", siteItemJson(&api))
			if got != nil || !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), c.want) {
				t.Errorf("fromJson = %v, %v; want an error that says %q", got, diags, c.want)
			}
		})
	}
}

// Every request carries the fields a configuration sets, and leaves out what P0 sets.
// Reading the stored item gives the model back.
func TestPoolAccountJson(t *testing.T) {
	model := &poolAccountModel{
		Id:         types.StringValue("agent-1"),
		Site:       types.StringValue("acme"),
		Email:      types.StringValue("agent-1@serviceaccount.atlassian.com"),
		AccountId:  types.StringValue("712020:abc"),
		SecretName: types.StringValue("p0_install_jira-ai_acme_agent-1_api-key"),
		State:      types.StringValue("installed"),
	}
	body := marshal(t, poolAccountToJson(model))
	want := `{"site":"acme","email":"agent-1@serviceaccount.atlassian.com"}`
	if body != want {
		t.Errorf("body = %s; want %s", body, want)
	}

	var api poolAccountApi
	stored := `{"item":` + strings.TrimSuffix(want, "}") + `,"accountId":"712020:abc","secretName":"p0_install_jira-ai_acme_agent-1_api-key","state":"installed"}}`
	if err := json.Unmarshal([]byte(stored), &api); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var diags diag.Diagnostics
	if got := poolAccountFromJson(context.Background(), &diags, "agent-1", poolAccountItemJson(&api)); diags.HasError() || !reflect.DeepEqual(got, model) {
		t.Errorf("fromJson = %+v, %v; want %+v", got, diags, model)
	}
}

func TestDescribeCheckError(t *testing.T) {
	summary, detail := describeCheckError("site")("acme", errTest("P0 couldn't look up the cloud ID of acme.atlassian.net."))
	if summary != "Jira site install check failed" || !strings.Contains(detail, `"acme"`) || !strings.Contains(detail, "couldn't look up the cloud ID") {
		t.Errorf("describeCheckError = %q, %q", summary, detail)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
