package installtailscale

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A destination of one type, as Terraform and as P0's API hold it.
type destinationCase struct {
	name string
	// The destination's configuration, with every field it doesn't set null.
	model func() *tailscaleDestinationModel
	// The body of every request that carries the destination: its tailnet and its kind.
	body string
}

// A destination model on the test tailnet, keyed prod-db, of the given type and fields.
func destinationOf(destinationType, target string, set func(*tailscaleDestinationModel)) func() *tailscaleDestinationModel {
	return func() *tailscaleDestinationModel {
		model := &tailscaleDestinationModel{
			Id:      types.StringValue(testDestination),
			Tailnet: types.StringValue(testTailnet),
			Name:    types.StringValue("prod-db"),
			Type:    types.StringValue(destinationType),
			Target:  types.StringValue(target),
			Ports:   types.StringNull(),
			SiteId:  types.StringNull(),
			Via:     types.StringNull(),
			Users:   types.StringNull(),
			Groups:  types.StringNull(),
			Proxy:   types.StringNull(),
			State:   types.StringNull(),
		}
		if set != nil {
			set(model)
		}
		return model
	}
}

var ipDestination = destinationCase{
	name: "ip",
	model: destinationOf(ipType, "10.0.4.0/24", func(m *tailscaleDestinationModel) {
		m.Ports = types.StringValue("tcp:5432")
		m.Via = types.StringValue("tag:prod-router")
	}),
	body: `{"tailnet":"T1234CNTRL","kind":{"type":"ip","target":"10.0.4.0/24","ports":"tcp:5432","via":"tag:prod-router"}}`,
}

var siteDestination = destinationCase{
	name: "ip behind a 4via6 site",
	model: destinationOf(ipType, "10.0.4.0/24", func(m *tailscaleDestinationModel) {
		m.SiteId = types.StringValue("7")
		m.Ports = types.StringValue("tcp:5432, tcp:6432")
	}),
	body: `{"tailnet":"T1234CNTRL","kind":{"type":"ip","target":"10.0.4.0/24","siteId":"7","ports":"tcp:5432, tcp:6432"}}`,
}

var sshDestination = destinationCase{
	name: "ssh",
	model: destinationOf(sshType, "tag:prod", func(m *tailscaleDestinationModel) {
		m.Users = types.StringValue("root,autogroup:nonroot")
	}),
	body: `{"tailnet":"T1234CNTRL","kind":{"type":"ssh","target":"tag:prod","users":"root,autogroup:nonroot"}}`,
}

// P0 stores the proxy's default for a Kubernetes API destination that leaves it out,
// and the plan has it too.
var capabilityDestination = destinationCase{
	name: "capability",
	model: destinationOf(capabilityType, "tag:k8s-operator", func(m *tailscaleDestinationModel) {
		m.Groups = types.StringValue("system:masters")
		m.Proxy = types.StringValue(operatorProxy)
	}),
	body: `{"tailnet":"T1234CNTRL","kind":{"type":"capability","target":"tag:k8s-operator","groups":"system:masters","proxy":{"type":"operator"}}}`,
}

var proxyGroupDestination = destinationCase{
	name: "capability through a ProxyGroup",
	model: destinationOf(capabilityType, "tag:k8s-api", func(m *tailscaleDestinationModel) {
		m.Groups = types.StringValue("system:masters")
		m.Proxy = types.StringValue(proxyGroupProxy)
	}),
	body: `{"tailnet":"T1234CNTRL","kind":{"type":"capability","target":"tag:k8s-api","groups":"system:masters","proxy":{"type":"proxy-group"}}}`,
}

var postureDestination = destinationCase{
	name:  "posture",
	model: destinationOf(postureType, "posture:p0_prod_db", nil),
	body:  `{"tailnet":"T1234CNTRL","kind":{"type":"posture","target":"posture:p0_prod_db"}}`,
}

var destinationCases = []destinationCase{ipDestination, siteDestination, sshDestination, capabilityDestination, proxyGroupDestination, postureDestination}

func (c destinationCase) destination(state string) *tailscaleDestinationModel {
	model := c.model()
	model.State = types.StringValue(state)
	return model
}

// The plan for a new destination: its key and its state are unknown.
func (c destinationCase) planned() *tailscaleDestinationModel {
	model := c.model()
	model.Id = types.StringUnknown()
	model.State = types.StringUnknown()
	return model
}

// The item as P0's API answers at state.
func (c destinationCase) response(state string) string {
	return c.body[:len(c.body)-1] + `,"state":"` + state + `"}`
}

// Every request carries the tailnet and every field of the kind, in the field names of
// the app's install schema: `kind` is a select element whose option is its "type", and
// the proxy is one too. None carries what P0 assigns.
func TestDestinationRequestJson(t *testing.T) {
	for _, c := range destinationCases {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&TailscaleDestination{}).toJson(c.planned())); got != c.body {
				t.Errorf("request body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&TailscaleDestination{}).toJson(c.destination(common.StateInstalled))); got != c.body {
				t.Errorf("request body of an installed destination =\n%s\nwant\n%s", got, c.body)
			}
		})
	}

	// A Kubernetes API destination always sends its proxy: P0 refuses a request without
	// the one it stored.
	planned := capabilityDestination.planned()
	planned.Proxy = types.StringUnknown()
	if got := marshal(t, (&TailscaleDestination{}).toJson(planned)); got != capabilityDestination.body {
		t.Errorf("request body without a known proxy =\n%s\nwant\n%s", got, capabilityDestination.body)
	}
}

// Reading back what P0 stores gives the model that was sent, with the tailnet and the
// name from the key, plus the state.
func TestDestinationFromJson(t *testing.T) {
	for _, c := range destinationCases {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var item tailscaleDestinationJson
				if err := json.Unmarshal([]byte(c.response(state)), &item); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}

				var diags diag.Diagnostics
				got, ok := (&TailscaleDestination{}).fromJson(context.Background(), &diags, testDestination, &item).(*tailscaleDestinationModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.destination(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}
				rawOf(t, NewTailscaleDestination(), got)
			})
		}
	}
}

// A Kubernetes API destination without a stored proxy reads as the default, which is
// what the connector uses. Any other type has none.
func TestDestinationFromJsonProxy(t *testing.T) {
	cases := map[string]struct {
		kind kindJson
		want types.String
	}{
		"capability without one": {kind: kindJson{Type: capabilityType}, want: types.StringValue(operatorProxy)},
		"ip with one":            {kind: kindJson{Type: ipType, Proxy: &selectJson{Type: proxyGroupProxy}}, want: types.StringNull()},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got, ok := (&TailscaleDestination{}).fromJson(context.Background(), &diags, testDestination, &tailscaleDestinationJson{Kind: &c.kind}).(*tailscaleDestinationModel)
			if !ok || diags.HasError() {
				t.Fatalf("fromJson failed: %v", diags)
			}
			if !got.Proxy.Equal(c.want) {
				t.Errorf("proxy = %v; want %v", got.Proxy, c.want)
			}
		})
	}
}

// The installer's form stores "" for an optional field an admin typed in and cleared.
// It reads as null, as a field left out does, so an imported destination whose
// configuration leaves the field out plans no replacement.
func TestDestinationFromJsonEmptyOptionalFields(t *testing.T) {
	empty, router := "", "tag:prod-router"
	cases := map[string]struct {
		kind kindJson
		want *tailscaleDestinationModel
	}{
		"ip with an empty site and router": {
			kind: kindJson{Type: ipType, SiteId: &empty, Via: &empty},
			want: &tailscaleDestinationModel{SiteId: types.StringNull(), Via: types.StringNull(), Users: types.StringNull()},
		},
		"ip with a router": {
			kind: kindJson{Type: ipType, SiteId: &empty, Via: &router},
			want: &tailscaleDestinationModel{SiteId: types.StringNull(), Via: types.StringValue(router), Users: types.StringNull()},
		},
		"ssh with empty users": {
			kind: kindJson{Type: sshType, Users: &empty},
			want: &tailscaleDestinationModel{SiteId: types.StringNull(), Via: types.StringNull(), Users: types.StringNull()},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got, ok := (&TailscaleDestination{}).fromJson(context.Background(), &diags, testDestination, &tailscaleDestinationJson{Kind: &c.kind}).(*tailscaleDestinationModel)
			if !ok || diags.HasError() {
				t.Fatalf("fromJson failed: %v", diags)
			}
			if !got.SiteId.Equal(c.want.SiteId) || !got.Via.Equal(c.want.Via) || !got.Users.Equal(c.want.Users) {
				t.Errorf("site_id, via, users = %v, %v, %v; want %v, %v, %v", got.SiteId, got.Via, got.Users, c.want.SiteId, c.want.Via, c.want.Users)
			}
		})
	}
}

// A key that isn't a destination's, an item without a kind, or a type this provider
// doesn't know is refused rather than read into the wrong fields.
func TestDestinationFromJsonRefuses(t *testing.T) {
	cases := map[string]struct {
		key  string
		item tailscaleDestinationJson
		want string
	}{
		"a bad key":    {key: "prod-db", item: tailscaleDestinationJson{Kind: &kindJson{Type: ipType}}, want: "Bad destination key"},
		"no kind":      {key: testDestination, want: "Bad API response"},
		"unknown type": {key: testDestination, item: tailscaleDestinationJson{Kind: &kindJson{Type: "funnel"}}, want: "Unsupported Tailscale destination"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := (&TailscaleDestination{}).fromJson(context.Background(), &diags, c.key, &c.item)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != c.want {
				t.Errorf("fromJson = %+v, %v; want nil and %q", got, diags, c.want)
			}
		})
	}
}

// P0 fixes every field of a destination when it adds the destination, so changing any
// of them replaces it. What P0 assigns is read-only, and the proxy is computed too, for
// the default P0 stores.
func TestDestinationSchema(t *testing.T) {
	s := schemaOf(t, NewTailscaleDestination())
	for _, name := range []string{"tailnet", "name", "type", "target", "ports", "site_id", "via", "users", "groups", "proxy"} {
		if !requiresReplace(t, s.Attributes[name]) {
			t.Errorf("%s doesn't require replacement", name)
		}
	}
	for _, name := range []string{"tailnet", "name", "type", "target"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s isn't required", name)
		}
	}
	for _, name := range []string{"id", "state"} {
		attribute := s.Attributes[name]
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s = %+v; want a read-only attribute", name, attribute)
		}
	}
	if proxy := s.Attributes["proxy"]; !proxy.IsOptional() || !proxy.IsComputed() {
		t.Errorf("proxy = %+v; want an optional, computed attribute", proxy)
	}
}

// The name rule (DESTINATION_NAME_PATTERN), and P0's words for a name that breaks it. A
// name goes into P0's URL paths unescaped, so nothing a URL reads differently gets in.
func TestDestinationNameValidator(t *testing.T) {
	cases := map[string]bool{
		"prod-db":                     true,
		"a":                           true,
		"0":                           true,
		"web.v2_blue-green":           true,
		"A" + strings.Repeat("b", 62): true,
		"A" + strings.Repeat("b", 63): false,
		"-prod":                       false,
		".prod":                       false,
		"prod db":                     false,
		"prod:db":                     false,
		"prod/db":                     false,
		"prod#db":                     false,
		"prod?db":                     false,
		"prod%41":                     false,
		"prod\n":                      false,
		"":                            false,
	}

	attribute := schemaOf(t, NewTailscaleDestination()).Attributes["name"]
	for name, want := range cases {
		if got := accepts(t, attribute, name); got != want {
			t.Errorf("name accepts %q = %v; want %v", name, got, want)
		}
	}
	const message = "The name takes letters, digits, '.', '_' and '-', up to 63 characters, starting with a letter or digit."
	if got := errorDetails(validateString(t, attribute, "prod db")); len(got) != 1 || !strings.HasSuffix(got[0], message) {
		t.Errorf("name refuses a space with %v; want P0's message alone", got)
	}
}

// The attribute validators take the types and proxies the app offers.
func TestDestinationTypeValidators(t *testing.T) {
	s := schemaOf(t, NewTailscaleDestination())
	for _, destinationType := range destinationTypes {
		if !accepts(t, s.Attributes["type"], destinationType) {
			t.Errorf("type refuses %q", destinationType)
		}
	}
	for value, want := range map[string]bool{"funnel": false, "IP": false, "": false} {
		if got := accepts(t, s.Attributes["type"], value); got != want {
			t.Errorf("type accepts %q = %v; want %v", value, got, want)
		}
	}
	for value, want := range map[string]bool{"operator": true, "proxy-group": true, "proxygroup": false} {
		if got := accepts(t, s.Attributes["proxy"], value); got != want {
			t.Errorf("proxy accepts %q = %v; want %v", value, got, want)
		}
	}
}

// Runs the destination's ValidateConfig against a configuration of the type, with the
// given fields and every other one null.
func validateDestination(t *testing.T, destinationType string, fields map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	values := map[string]tftypes.Value{"tailnet": str(testTailnet), "name": str("prod-db"), "type": str(destinationType)}
	for name, value := range fields {
		values[name] = value
	}
	return validateConfig(t, &TailscaleDestination{}, nil, values)
}

// P0's rules for each type (packages/integrations/tailscale/backend/src/destination.ts
// in the app, and the connector's rules it checks by), run through ValidateConfig:
// which fields each type takes and needs, and the form of each value. Each refused
// value gets one error, at its attribute, with P0's message. Each check skips a value
// Terraform doesn't know yet.
func TestDestinationValidateConfig(t *testing.T) {
	const (
		tag     = "The target must be a tag, such as tag:prod."
		service = "The target must be a service, such as svc:postgres."
		nodeId  = "The target must be a Tailscale node id."
		posture = "The target must be a P0 posture, such as posture:p0_prod_db."
		port    = "Each port must be a port such as tcp:5432, a range such as tcp:8000-8100, or *."
		user    = "Each SSH user must be an SSH user."
		group   = "Each Kubernetes group must be a Kubernetes group."
		router  = "The router tag must be a tag, such as tag:prod."
	)
	padded := func(field string) string {
		return field + ": " + strings.ReplaceAll(paddedValue, "%s", field)
	}
	fields := func(pairs ...string) map[string]tftypes.Value {
		values := map[string]tftypes.Value{}
		for i := 0; i < len(pairs); i += 2 {
			values[pairs[i]] = str(pairs[i+1])
		}
		return values
	}

	cases := []struct {
		name            string
		destinationType string
		fields          map[string]tftypes.Value
		want            []string
	}{
		// Every type, as the app's descriptions give it.
		{name: "ip", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "tcp:5432", "via", "tag:prod-router")},
		{name: "ip behind a site", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "site_id", "65535", "ports", "*")},
		{name: "ipv6", destinationType: ipType, fields: fields("target", "fd7a:115c:a1e0::/48", "ports", "tcp:22,udp:53")},
		{name: "device", destinationType: deviceType, fields: fields("target", "nAbc123CNTRL", "ports", "tcp:443")},
		{name: "tag", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "tcp:8000-8100, icmp:*")},
		{name: "service", destinationType: serviceType, fields: fields("target", "svc:postgres", "ports", "tcp:5432")},
		{name: "ssh", destinationType: sshType, fields: fields("target", "tag:prod")},
		{name: "ssh with users", destinationType: sshType, fields: fields("target", "tag:prod", "users", "root, autogroup:nonroot")},
		{name: "app connector", destinationType: appConnectorType, fields: fields("target", "github")},
		{name: "capability", destinationType: capabilityType, fields: fields("target", "tag:k8s-operator", "groups", "system:masters")},
		{name: "capability through a ProxyGroup", destinationType: capabilityType, fields: fields("target", "tag:k8s-api", "groups", "system:masters,dev@example.com", "proxy", "proxy-group")},
		{name: "exit node", destinationType: exitNodeType, fields: fields("target", "nAbc123CNTRL")},
		{name: "posture", destinationType: postureType, fields: fields("target", "posture:p0_prod_db")},
		// P0 takes an empty optional field as one left out, and fromJson reads it back as
		// null, so the configuration leaves it out.
		{
			name: "empty optional fields", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "tcp:5432", "site_id", "", "via", ""),
			want: []string{
				`site_id: Leave 'site_id' out rather than setting it to "": P0 takes an empty value as one left out, and Terraform would read it back as null.`,
				`via: Leave 'via' out rather than setting it to "": P0 takes an empty value as one left out, and Terraform would read it back as null.`,
			},
		},
		{
			name: "empty users", destinationType: sshType, fields: fields("target", "tag:prod", "users", ""),
			want: []string{`users: Leave 'users' out rather than setting it to "": P0 takes an empty value as one left out, and Terraform would read it back as null.`},
		},

		// Values Terraform doesn't know yet.
		{name: "unknown type", destinationType: "", fields: map[string]tftypes.Value{"type": unknownString, "target": str("x"), "groups": str("system:masters")}},
		{name: "unknown target", destinationType: tagType, fields: map[string]tftypes.Value{"target": unknownString, "ports": str("tcp:22")}},
		{name: "unknown ports", destinationType: tagType, fields: map[string]tftypes.Value{"target": str("tag:prod"), "ports": unknownString}},
		{name: "unknown users on ip", destinationType: ipType, fields: map[string]tftypes.Value{"target": str("10.0.4.0/24"), "ports": str("*"), "users": unknownString}},

		// The fields each type takes.
		{
			name: "ports on ssh", destinationType: sshType, fields: fields("target", "tag:prod", "ports", "tcp:22"),
			want: []string{`ports: 'ports' may only be set when 'type' is "ip", "device", "tag" or "service".`},
		},
		{
			name: "a site on a tag", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "*", "site_id", "7"),
			want: []string{`site_id: 'site_id' may only be set when 'type' is "ip".`},
		},
		{
			name: "via on a device", destinationType: deviceType, fields: fields("target", "nAbc123CNTRL", "ports", "*", "via", "tag:router"),
			want: []string{`via: 'via' may only be set when 'type' is "ip".`},
		},
		{
			name: "users on a tag", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "*", "users", "root"),
			want: []string{`users: 'users' may only be set when 'type' is "ssh".`},
		},
		{
			name: "groups on ssh", destinationType: sshType, fields: fields("target", "tag:prod", "groups", "system:masters"),
			want: []string{`groups: 'groups' may only be set when 'type' is "capability".`},
		},
		{
			name: "a proxy on an exit node", destinationType: exitNodeType, fields: fields("target", "nAbc123CNTRL", "proxy", "operator"),
			want: []string{`proxy: 'proxy' may only be set when 'type' is "capability".`},
		},
		{
			name: "every other field on a posture", destinationType: postureType,
			fields: fields("target", "posture:p0_prod_db", "ports", "*", "users", "root"),
			want: []string{
				`ports: 'ports' may only be set when 'type' is "ip", "device", "tag" or "service".`,
				`users: 'users' may only be set when 'type' is "ssh".`,
			},
		},

		// The fields each type needs.
		{name: "ip without ports", destinationType: ipType, fields: fields("target", "10.0.4.0/24"), want: []string{"ports: " + missingPorts}},
		{name: "a service without ports", destinationType: serviceType, fields: fields("target", "svc:postgres"), want: []string{"ports: " + missingPorts}},
		{name: "ports with no port", destinationType: deviceType, fields: fields("target", "nAbc123CNTRL", "ports", " , "), want: []string{"ports: " + missingPorts}},
		{name: "capability without groups", destinationType: capabilityType, fields: fields("target", "tag:k8s-operator"), want: []string{"groups: " + missingGroups}},
		{name: "groups with no group", destinationType: capabilityType, fields: fields("target", "tag:k8s-operator", "groups", ","), want: []string{"groups: " + missingGroups}},
		{name: "an empty target", destinationType: appConnectorType, fields: fields("target", ""), want: []string{"target: " + missingTarget}},
		{name: "a blank target", destinationType: tagType, fields: fields("target", "  ", "ports", "*"), want: []string{"target: " + missingTarget}},

		// The form of each type's target.
		{name: "a tag without tag:", destinationType: tagType, fields: fields("target", "prod", "ports", "*"), want: []string{"target: " + tag}},
		{name: "a tag starting with a digit", destinationType: sshType, fields: fields("target", "tag:1prod"), want: []string{"target: " + tag}},
		{name: "an operator's tag with an underscore", destinationType: capabilityType, fields: fields("target", "tag:k8s_operator", "groups", "system:masters"), want: []string{"target: " + tag}},
		{name: "a service without svc:", destinationType: serviceType, fields: fields("target", "postgres", "ports", "*"), want: []string{"target: " + service}},
		{name: "a device by its name", destinationType: deviceType, fields: fields("target", "db-1.tail1234.ts.net", "ports", "*"), want: []string{"target: " + nodeId}},
		{name: "an exit node by its IP", destinationType: exitNodeType, fields: fields("target", "100.64.0.1"), want: []string{"target: " + nodeId}},
		{name: "a posture that isn't P0's", destinationType: postureType, fields: fields("target", "posture:prod_db"), want: []string{"target: " + posture}},
		{name: "a posture with a hyphen", destinationType: postureType, fields: fields("target", "posture:p0_prod-db"), want: []string{"target: " + posture}},

		// The form of each other field.
		{name: "a port without a protocol's colon", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "tcp5432"), want: []string{"ports: " + port}},
		{name: "one bad port among good ones", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "tcp:22, tcp:ssh"), want: []string{"ports: " + port}},
		{name: "a site that isn't a number", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "*", "site_id", "seven"), want: []string{"site_id: " + invalidSiteId}},
		{name: "a site past 65535", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "*", "site_id", "65536"), want: []string{"site_id: " + invalidSiteId}},
		{name: "a negative site", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "*", "site_id", "-1"), want: []string{"site_id: " + invalidSiteId}},
		{name: "a router that isn't a tag", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "*", "via", "router-1"), want: []string{"via: " + router}},
		{name: "an SSH user with a space", destinationType: sshType, fields: fields("target", "tag:prod", "users", "root, deploy user"), want: []string{"users: " + user}},
		{name: "a Kubernetes group with a space", destinationType: capabilityType, fields: fields("target", "tag:k8s-operator", "groups", "system masters"), want: []string{"groups: " + group}},

		// P0 checks a value trimmed, and stores it trimmed, so a padded value that's valid
		// otherwise is refused for its whitespace alone.
		{name: "a padded target", destinationType: tagType, fields: fields("target", " tag:prod", "ports", "*"), want: []string{padded("target")}},
		{name: "padded ports", destinationType: tagType, fields: fields("target", "tag:prod", "ports", "tcp:22 "), want: []string{padded("ports")}},
		{name: "a padded site", destinationType: ipType, fields: fields("target", "10.0.4.0/24", "ports", "*", "site_id", "7\n"), want: []string{padded("site_id")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorDetails(validateDestination(t, c.destinationType, c.fields))
			want := append([]string{}, c.want...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("errors =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Every field a type's rules name is an attribute, and every field that some types
// take is named by at least one type. A misspelt name would read a null value, which
// skips the check silently.
func TestDestinationRuleFieldNames(t *testing.T) {
	attributes := schemaOf(t, NewTailscaleDestination()).Attributes
	values := ipDestination.model().fields()
	for destinationType, fields := range destinationRules {
		for _, field := range fields {
			if _, ok := attributes[field.name]; !ok {
				t.Errorf("rule for %s names %q, which the resource doesn't have", destinationType, field.name)
			}
			if _, ok := values[field.name]; !ok {
				t.Errorf("rule for %s names %q, which the model's fields don't have", destinationType, field.name)
			}
		}
	}
	for _, field := range optionalDestinationFields {
		if typesTaking(field.name) == "" {
			t.Errorf("no type takes %q", field.name)
		}
	}
	for _, destinationType := range destinationTypes {
		if len(destinationRules[destinationType]) == 0 {
			t.Errorf("no rules for %s", destinationType)
		}
	}
}

// A Kubernetes API destination that leaves its proxy out plans P0's default, and any
// other type plans none, so taking `proxy` out of the configuration replaces a
// ProxyGroup destination with an operator one. A proxy the configuration sets is kept,
// and one whose type Terraform doesn't know yet stays unknown.
func TestProxyDefault(t *testing.T) {
	r := NewTailscaleDestination()
	s := schemaOf(t, r)
	planOf := func(destinationType types.String) tfsdk.Plan {
		model := capabilityDestination.planned()
		model.Type = destinationType
		model.Proxy = types.StringUnknown()
		return tfsdk.Plan{Schema: s, Raw: rawOf(t, r, model)}
	}

	cases := []struct {
		name            string
		destinationType types.String
		config          types.String
		want            types.String
	}{
		{name: "capability without a proxy", destinationType: types.StringValue(capabilityType), config: types.StringNull(), want: types.StringValue(operatorProxy)},
		{name: "capability with a proxy", destinationType: types.StringValue(capabilityType), config: types.StringValue(proxyGroupProxy), want: types.StringUnknown()},
		{name: "ip", destinationType: types.StringValue(ipType), config: types.StringNull(), want: types.StringNull()},
		{name: "an unknown type", destinationType: types.StringUnknown(), config: types.StringNull(), want: types.StringUnknown()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planmodifier.StringRequest{Path: path.Root("proxy"), Plan: planOf(c.destinationType), ConfigValue: c.config, PlanValue: types.StringUnknown()}
			resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
			proxyDefault{}.PlanModifyString(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("PlanModifyString: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(c.want) {
				t.Errorf("plan = %v; want %v", resp.PlanValue, c.want)
			}
		})
	}
}

// Creating a destination stages it with every field, then verifies and configures it,
// which is when P0 checks it through the connector.
func TestDestinationCreate(t *testing.T) {
	for _, c := range destinationCases {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &TailscaleDestination{}, server)

			resp := create(t, r, rawOf(t, r, c.planned()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake,
				"POST "+basePath+" {}",
				"PUT "+destinationPath+" "+c.body,
				"POST "+destinationPath+"/verify "+c.body,
				"POST "+destinationPath+"/configure "+c.body,
			)
			if want := rawOf(t, r, c.destination(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// When the check fails, the apply fails with the staged item in state, which Terraform
// taints, and the next apply replaces it.
func TestDestinationCreateFailedCheck(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.failVerify = true
	r := configure(t, &TailscaleDestination{}, server)

	resp := create(t, r, rawOf(t, r, ipDestination.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Tailscale destination install check failed" {
		t.Fatalf("diagnostics = %v; want the failed check", resp.Diagnostics)
	}
	wantRequests(t, fake,
		"POST "+basePath+" {}",
		"PUT "+destinationPath+" "+ipDestination.body,
		"POST "+destinationPath+"/verify "+ipDestination.body,
	)
	if want := rawOf(t, r, ipDestination.destination(common.StateStage)); !resp.State.Raw.Equal(want) {
		t.Errorf("state =\n%v\nwant the staged destination", resp.State.Raw)
	}
}

// P0 refuses to add a destination twice, so one that exists is imported, not created.
func TestDestinationCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, destinationPath, common.StateInstalled, json.RawMessage(ipDestination.body))
	r := configure(t, &TailscaleDestination{}, server)

	resp := create(t, r, rawOf(t, r, ipDestination.planned()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage destination component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(destinationPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

func TestDestinationDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, destinationPath, common.StateInstalled, json.RawMessage(ipDestination.body))
	r := configure(t, &TailscaleDestination{}, server)

	resp := destroy(t, r, rawOf(t, r, ipDestination.destination(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+destinationPath)
	if got := fake.State(destinationPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// A destination that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Every other plan is the
// framework's, so an installed destination plans no difference.
func TestDestinationModifyPlan(t *testing.T) {
	r := &TailscaleDestination{}
	at := func(state string) tftypes.Value { return rawOf(t, r, ipDestination.destination(state)) }
	finishing := func(state string) tftypes.Value {
		model := ipDestination.destination(state)
		model.State = types.StringUnknown()
		return rawOf(t, r, model)
	}
	none := rawOf(t, r, (*tailscaleDestinationModel)(nil))

	cases := []struct {
		name  string
		prior tftypes.Value
		plan  tftypes.Value
		want  tftypes.Value
	}{
		{name: "installed", prior: at(common.StateInstalled), plan: at(common.StateInstalled), want: at(common.StateInstalled)},
		{name: "configure", prior: at(common.StateConfigure), plan: at(common.StateConfigure), want: finishing(common.StateConfigure)},
		{name: "stage", prior: at(common.StateStage), plan: at(common.StateStage), want: finishing(common.StateStage)},
		{name: "create", prior: none, plan: rawOf(t, r, ipDestination.planned()), want: rawOf(t, r, ipDestination.planned())},
		{name: "destroy", prior: at(common.StateInstalled), plan: none, want: none},
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

// A destination imported at stage converges in one apply: its plan shows an update,
// the update verifies and configures it without staging it, and the plan after that
// shows no difference.
func TestDestinationStagedItemConverges(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, destinationPath, common.StateStage, json.RawMessage(capabilityDestination.body))
	r := configure(t, &TailscaleDestination{}, server)

	staged := rawOf(t, r, capabilityDestination.destination(common.StateStage))
	plan := modifyPlan(t, r, staged, staged)
	if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
		t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
	}

	resp := update(t, r, staged, plan.Plan.Raw)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "POST "+destinationPath+"/verify "+capabilityDestination.body, "POST "+destinationPath+"/configure "+capabilityDestination.body)
	installed := rawOf(t, r, capabilityDestination.destination(common.StateInstalled))
	if !resp.State.Raw.Equal(installed) {
		t.Fatalf("state =\n%v\nwant the installed destination", resp.State.Raw)
	}
	if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
		t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
	}
}

// A destination is imported by its key, whose halves get the configuration's checks
// before the read puts the key in a URL path. The import sets the key's tailnet and
// name, and the read fills in the rest.
func TestDestinationImportState(t *testing.T) {
	r := &TailscaleDestination{}
	s := schemaOf(t, r)
	importKey := func(key string) *resource.ImportStateResponse {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: key}, resp)
		return resp
	}

	resp := importKey(testDestination)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics)
	}
	var got tailscaleDestinationModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}
	if got.Id.ValueString() != testDestination || got.Tailnet.ValueString() != testTailnet || got.Name.ValueString() != "prod-db" {
		t.Errorf("imported = %+v; want the key's tailnet and name", got)
	}

	for _, key := range []string{"prod-db", "T1234CNTRL:prod#db", "tail1234.ts.net:prod-db", "T1234CNTRL:%zz"} {
		resp := importKey(key)
		if errs := resp.Diagnostics.Errors(); len(errs) != 1 || errs[0].Detail() != invalidDestinationKey {
			t.Errorf("import %q = %v; want it refused with P0's message", key, resp.Diagnostics)
		}
	}
}

// The plan's computed key is unknown until P0 has the item, but the key is known from
// the tailnet and the name, so every request goes to the item's path.
func TestDestinationGetIdIsTheKey(t *testing.T) {
	if got := (&TailscaleDestination{}).getId(ipDestination.planned()); got == nil || *got != testDestination {
		t.Errorf("getId = %v; want %q", got, testDestination)
	}
}
