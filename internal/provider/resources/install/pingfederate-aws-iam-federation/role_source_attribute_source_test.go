package installpingfederateawsiamfederation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

const (
	testEntityId = "urn:example:pingfederate"
	testIssuer   = "urn:example:pingfederate:p0"
	testClientId = "p0-roles"
	testJwks     = `{"keys":[{"kty":"RSA","e":"AQAB","n":"x"}]}`
)

// An attribute-source app, as Terraform holds it once P0 has the item. jwks is null
// when P0 reads PingFederate's JWKS endpoint.
func attributeSourceApp(state string, issuer string, jwks types.String) *pingFederateAwsIamFederationAppModel {
	return &pingFederateAwsIamFederationAppModel{
		Id:              types.StringValue(testAppId),
		PingFederateUrl: types.StringValue(testPingFederateUrl),
		AttributeSource: &attributeSourceModel{
			EntityId: types.StringValue(testEntityId),
			Issuer:   types.StringValue(issuer),
			ClientId: types.StringValue(testClientId),
			Jwks:     jwks,
			Audience: types.StringValue(fakeAudience),
			RolesUrl: types.StringNull(),
		},
		SamlMetadataUrl: types.StringValue(fakeMetadataUrl),
		State:           types.StringValue(state),
	}
}

// The body of every request that carries an attribute-source app. It never carries
// the audience, which P0 sets.
func attributeSourceBody(t *testing.T, issuer string, jwks types.String) string {
	t.Helper()
	keys := jwksJson{Type: jwksFromUrl}
	if !jwks.IsNull() {
		keys = jwksJson{Type: jwksFromKeys, Keys: jwks.ValueStringPointer()}
	}
	return `{"pingFederateUrl":"` + testPingFederateUrl + `","roleSource":{"type":"attribute-source",` +
		`"entityId":"` + testEntityId + `","issuer":"` + issuer + `","clientId":"` + testClientId + `","jwks":` + marshal(t, keys) + `}}`
}

var jwksCases = []struct {
	name string
	jwks types.String
}{
	{name: "from the JWKS endpoint", jwks: types.StringNull()},
	{name: "pasted", jwks: types.StringValue(testJwks)},
}

func TestAttributeSourceRequestJson(t *testing.T) {
	for _, c := range jwksCases {
		t.Run(c.name, func(t *testing.T) {
			got := marshal(t, (&PingFederateAwsIamFederationApp{}).toJson(attributeSourceApp(common.StateInstalled, testIssuer, c.jwks)))
			if want := attributeSourceBody(t, testIssuer, c.jwks); got != want {
				t.Errorf("request body =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestAttributeSourceFromJson(t *testing.T) {
	for _, c := range jwksCases {
		t.Run(c.name, func(t *testing.T) {
			body := attributeSourceBody(t, testIssuer, c.jwks)
			response := `{"item":` + body[:len(body)-2] + `,"audience":"` + fakeAudience + `"},"state":"installed"},` +
				`"metadata":{"samlMetadataUrl":"` + fakeMetadataUrl + `"}}`
			var item pingFederateAwsIamFederationAppApi
			if err := json.Unmarshal([]byte(response), &item); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			var diags diag.Diagnostics
			got := (&PingFederateAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item)
			if diags.HasError() {
				t.Fatalf("fromJson: %v", diags)
			}
			if want := attributeSourceApp(common.StateInstalled, testIssuer, c.jwks); !reflect.DeepEqual(got, want) {
				t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
			}
			rawOf(t, NewPingFederateAwsIamFederationApp(), got)
		})
	}
}

// An app this provider can't represent is refused rather than read into the wrong
// block.
func TestAppFromJsonRefusesAnUnknownShape(t *testing.T) {
	cases := []struct {
		name       string
		roleSource *roleSourceJson
		summary    string
	}{
		{name: "no role source", summary: "Bad API response"},
		{name: "an unknown role source", roleSource: &roleSourceJson{Type: "ldap"}, summary: "Unsupported PingFederate (AWS IAM Federation) app"},
		{name: "unknown keys", roleSource: &roleSourceJson{Type: attributeSourceRoleSourceType, Jwks: &jwksJson{Type: "x5c"}}, summary: "Unsupported PingFederate (AWS IAM Federation) app"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			item := pingFederateAwsIamFederationAppApi{Item: &pingFederateAwsIamFederationAppJson{RoleSource: c.roleSource}}
			var diags diag.Diagnostics
			got := (&PingFederateAwsIamFederationApp{}).fromJson(context.Background(), &diags, testAppId, &item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != c.summary {
				t.Errorf("fromJson = %+v, %v; want nil and %q", got, diags, c.summary)
			}
		})
	}
}

func TestRolesUrlOf(t *testing.T) {
	cases := []struct {
		name      string
		installer *common.Install
		want      types.String
	}{
		{name: "an org", installer: &common.Install{ProviderData: &internal.P0ProviderData{BaseUrl: "https://api.p0.app/o/acme"}}, want: types.StringValue("https://api.p0.app/integrations/pingfederate-aws-iam-federation/acme")},
		{name: "no org", installer: &common.Install{ProviderData: &internal.P0ProviderData{BaseUrl: "https://api.p0.app"}}, want: types.StringNull()},
		{name: "no installer", want: types.StringNull()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rolesUrlOf(c.installer); !got.Equal(c.want) {
				t.Errorf("rolesUrlOf = %v; want %v", got, c.want)
			}
		})
	}
}

// P0's own cases for an attribute source's fields
// (packages/integrations/pingfederate-aws-iam-federation/backend/src/role-sources/attribute-source/__tests__/install.test.ts
// in the app), run through ValidateConfig.
func TestAttributeSourceValidateConfig(t *testing.T) {
	padded := func(field string) string {
		return "attribute_source." + field + ": P0 removes the whitespace around 'attribute_source." + field + "', so the value it stores " +
			"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."
	}
	cases := []struct {
		name  string
		field string
		value tftypes.Value
		want  []string
	}{
		{name: "keys from the JWKS endpoint", field: "jwks", value: tftypes.NewValue(tftypes.String, nil)},
		{name: "pasted keys", field: "jwks", value: str(testJwks)},
		{name: "unknown keys", field: "jwks", value: unknownString},
		{name: "an empty entity ID", field: "entity_id", value: str(""), want: []string{"attribute_source.entity_id: " + invalidEntityId}},
		{name: "a blank issuer", field: "issuer", value: str(" "), want: []string{"attribute_source.issuer: " + invalidIssuer}},
		{name: "a padded issuer", field: "issuer", value: str(testIssuer + "\n"), want: []string{padded("issuer")}},
		{name: "an empty client ID", field: "client_id", value: str(""), want: []string{"attribute_source.client_id: " + invalidClientId}},
		{name: "an empty key list", field: "jwks", value: str(`{"keys":[]}`), want: []string{"attribute_source.jwks: " + invalidJwks}},
		{name: "a key without kty", field: "jwks", value: str(`{"keys":[{"kid":"a"}]}`), want: []string{"attribute_source.jwks: " + invalidJwks}},
		{name: "keys that aren't JSON", field: "jwks", value: str("{"), want: []string{"attribute_source.jwks: " + invalidJwks}},
		{name: "keys with a trailing newline", field: "jwks", value: str(testJwks + "\n"), want: []string{padded("jwks")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fields := map[string]tftypes.Value{"entity_id": str(testEntityId), "issuer": str(testIssuer), "client_id": str(testClientId)}
			fields[c.field] = c.value
			blocks := map[string]func(tftypes.Object) tftypes.Value{"attribute_source": objectOf(fields)}
			got := errorDetails(validateConfig(t, &PingFederateAwsIamFederationApp{}, blocks, appValues()))
			if want := append([]string{}, c.want...); !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Creating an attribute-source app stages, verifies and configures it, and keeps the
// audience that P0 sets.
func TestAttributeSourceCreate(t *testing.T) {
	fake, server := newFakeItems(t)
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)
	planned := attributeSourceApp(common.StateStage, testIssuer, types.StringNull())
	planned.AttributeSource.Audience = types.StringUnknown()
	planned.AttributeSource.RolesUrl = types.StringUnknown()
	planned.SamlMetadataUrl = types.StringUnknown()
	planned.State = types.StringUnknown()

	resp := create(t, r, rawOf(t, r, planned))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	body := attributeSourceBody(t, testIssuer, types.StringNull())
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+appPath+" "+body,
		"POST "+appPath+"/verify "+body,
		"POST "+appPath+"/configure "+body,
	)
	if want := rawOf(t, r, attributeSourceApp(common.StateInstalled, testIssuer, types.StringNull())); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
	}
}

// A new issuer or pasted keys on an installed app is one configure, with no verify, so
// a refused value leaves the app as it was.
func TestAttributeSourceUpdateInPlace(t *testing.T) {
	const newIssuer = "urn:example:pingfederate:p0-2"
	fake, server := newFakeItems(t)
	fake.seed(t, appPath, common.StateInstalled, json.RawMessage(attributeSourceBody(t, testIssuer, types.StringNull())))
	r := configure(t, &PingFederateAwsIamFederationApp{}, server)
	prior := rawOf(t, r, attributeSourceApp(common.StateInstalled, testIssuer, types.StringNull()))
	planned := rawOf(t, r, attributeSourceApp(common.StateInstalled, newIssuer, types.StringValue(testJwks)))

	resp := update(t, r, prior, planned)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+appPath+"/configure "+attributeSourceBody(t, newIssuer, types.StringValue(testJwks)))
	if !resp.State.Raw.Equal(planned) {
		t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, planned)
	}
}

// Moving an app from one role source to the other replaces it, because P0 fixes the
// role source. A change inside an attribute source's block doesn't.
func TestRoleSourcePlanModifiers(t *testing.T) {
	r := NewPingFederateAwsIamFederationApp()
	s := schemaOf(t, r)
	// An update of the resource: a change to a block is planned only then.
	existing := rawOf(t, r, attributeSourceApp(common.StateInstalled, testIssuer, types.StringNull()))
	for _, name := range []string{"scim", "attribute_source"} {
		attribute := as[schema.SingleNestedAttribute](t, s.Attributes[name])
		objectType := as[tftypes.Object](t, attribute.GetType().TerraformType(context.Background()))
		set, _ := attribute.GetType().ValueFromTerraform(context.Background(), objectOf(nil)(objectType))
		null, _ := attribute.GetType().ValueFromTerraform(context.Background(), tftypes.NewValue(objectType, nil))
		cases := map[string]struct {
			state, plan any
			want        bool
		}{
			"added":   {state: null, plan: set, want: true},
			"removed": {state: set, plan: null, want: true},
			"kept":    {state: set, plan: set, want: false},
		}
		for change, c := range cases {
			resp := &planmodifier.ObjectResponse{}
			for _, modifier := range attribute.PlanModifiers {
				modifier.PlanModifyObject(context.Background(), planmodifier.ObjectRequest{
					Path:       path.Root(name),
					State:      tfsdk.State{Schema: s, Raw: existing},
					Plan:       tfsdk.Plan{Schema: s, Raw: existing},
					StateValue: as[types.Object](t, c.state),
					PlanValue:  as[types.Object](t, c.plan),
				}, resp)
			}
			if resp.RequiresReplace != c.want {
				t.Errorf("%s %s: requires replacement = %v; want %v", name, change, resp.RequiresReplace, c.want)
			}
		}
	}
}
