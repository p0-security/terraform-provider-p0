package installclickhouse

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// copy of internal/provider/resources/install/auth0/connector_test.go (#245),
// itself a copy of Docker Hub's, renamed for ClickHouse Cloud (INT-695).

// A connector in one cloud, as Terraform and as P0's API hold it.
type connectorCloud struct {
	name string
	// Where the connector runs, with the URL that P0 resolves when it verifies.
	hosting func(uri types.String) *hostingModel
	// The URL P0 resolves, which only a Cloud Run service has.
	uri      types.String
	identity types.Object
	// The body of every request that carries the connector: where it runs.
	body string
	// The item and its metadata, as P0's API answers at the given state.
	response func(state string) string
}

func identityOf(values map[string]attr.Value) types.Object {
	fields := map[string]attr.Value{}
	for name := range connectorIdentityAttrTypes {
		fields[name] = types.StringNull()
	}
	for name, value := range values {
		fields[name] = value
	}
	return types.ObjectValueMust(connectorIdentityAttrTypes, fields)
}

var awsCloud = connectorCloud{
	name: "aws",
	hosting: func(uri types.String) *hostingModel {
		return &hostingModel{
			Type:                types.StringValue(awsHosting),
			AccountId:           types.StringValue("123456789012"),
			ProjectId:           types.StringNull(),
			ConnectorName:       types.StringValue("p0-clickhouse-prod"),
			ConnectorRegion:     types.StringValue("us-west-2"),
			ConnectorServiceUri: uri,
		}
	},
	uri: types.StringNull(),
	identity: identityOf(map[string]attr.Value{
		"type":       types.StringValue("aws"),
		"account_id": types.StringValue("123456789012"),
		"role_arn":   types.StringValue("arn:aws:iam::123456789012:role/p0-clickhouse-prod"),
		"role_name":  types.StringValue("p0-clickhouse-prod"),
	}),
	body: `{"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-clickhouse-prod","connectorRegion":"us-west-2"}}`,
	response: func(state string) string {
		return `{"ok":true,"item":{"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-clickhouse-prod","connectorRegion":"us-west-2"},` +
			`"state":"` + state + `"},` +
			`"metadata":{"connectorIdentity":{"type":"aws","accountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/p0-clickhouse-prod","roleName":"p0-clickhouse-prod"}}}`
	},
}

var gcpCloud = connectorCloud{
	name: "gcp",
	hosting: func(uri types.String) *hostingModel {
		return &hostingModel{
			Type:                types.StringValue(gcpHosting),
			AccountId:           types.StringNull(),
			ProjectId:           types.StringValue("acme-prod"),
			ConnectorName:       types.StringValue("p0-clickhouse-prod"),
			ConnectorRegion:     types.StringValue("us-west1"),
			ConnectorServiceUri: uri,
		}
	},
	uri: types.StringValue(fakeServiceUri),
	identity: identityOf(map[string]attr.Value{
		"type":                  types.StringValue("gcp"),
		"project_id":            types.StringValue("acme-prod"),
		"service_account_email": types.StringValue("p0-clickhouse-prod@acme-prod.iam.gserviceaccount.com"),
		"service_account_id":    types.StringValue("p0-clickhouse-prod"),
	}),
	body: `{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-clickhouse-prod","connectorRegion":"us-west1"}}`,
	response: func(state string) string {
		uri := ""
		if state != common.StateStage {
			uri = `,"connectorServiceUri":"` + fakeServiceUri + `"`
		}
		return `{"ok":true,"item":{"hosting":{"type":"gcp","projectId":"acme-prod","connectorName":"p0-clickhouse-prod","connectorRegion":"us-west1"` + uri + `},` +
			`"state":"` + state + `"},` +
			`"metadata":{"connectorIdentity":{"type":"gcp","projectId":"acme-prod",` +
			`"serviceAccountEmail":"p0-clickhouse-prod@acme-prod.iam.gserviceaccount.com","serviceAccountId":"p0-clickhouse-prod"}}}`
	},
}

var connectorClouds = []connectorCloud{awsCloud, gcpCloud}

// The final resource's model for the connector at state, with what P0 has assigned by
// then.
func (c connectorCloud) connector(state string) *clickhouseConnectorModel {
	uri := types.StringNull()
	if state != common.StateStage {
		uri = c.uri
	}
	return &clickhouseConnectorModel{
		Id:                types.StringValue("prod"),
		Hosting:           c.hosting(uri),
		ConnectorIdentity: c.identity,
		State:             types.StringValue(state),
	}
}

// The final resource's plan for a new connector: everything P0 assigns is unknown.
func (c connectorCloud) plannedConnector() *clickhouseConnectorModel {
	return &clickhouseConnectorModel{
		Id:                types.StringValue("prod"),
		Hosting:           c.hosting(types.StringUnknown()),
		ConnectorIdentity: types.ObjectUnknown(connectorIdentityAttrTypes),
		State:             types.StringUnknown(),
	}
}

// The staged resource's model for the connector at state.
func (c connectorCloud) staged(state string) *clickhouseConnectorStagedModel {
	hosting := c.hosting(types.StringNull())
	return &clickhouseConnectorStagedModel{
		Id: types.StringValue("prod"),
		Hosting: &stagedHostingModel{
			Type:            hosting.Type,
			AccountId:       hosting.AccountId,
			ProjectId:       hosting.ProjectId,
			ConnectorName:   hosting.ConnectorName,
			ConnectorRegion: hosting.ConnectorRegion,
		},
		ConnectorIdentity: c.identity,
		State:             types.StringValue(state),
	}
}

func (c connectorCloud) plannedStaged() *clickhouseConnectorStagedModel {
	planned := c.staged("")
	planned.ConnectorIdentity = types.ObjectUnknown(connectorIdentityAttrTypes)
	planned.State = types.StringUnknown()
	return planned
}

func connectorApiOf(t *testing.T, response string) *clickhouseConnectorApi {
	t.Helper()
	var api clickhouseConnectorApi
	if err := json.Unmarshal([]byte(response), &api); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &api
}

// Every request that carries the connector says where it runs, in the field names of
// the app's install schema: `hosting` is the app integration's ConnectorHosting
// element. It never carries the Cloud Run URL, which P0 resolves, or anything else P0
// assigns.
func TestConnectorRequestJson(t *testing.T) {
	for _, c := range connectorClouds {
		t.Run(c.name, func(t *testing.T) {
			if got := marshal(t, (&ClickHouseConnectorStaged{}).toJson(c.plannedStaged())); got != c.body {
				t.Errorf("staged PUT body =\n%s\nwant\n%s", got, c.body)
			}
			if got := marshal(t, (&ClickHouseConnector{}).toJson(c.plannedConnector())); got != c.body {
				t.Errorf("verify and configure body =\n%s\nwant\n%s", got, c.body)
			}
			// The PUT that stages an installed connector again, when the final resource is
			// destroyed.
			if got := marshal(t, (&ClickHouseConnector{}).toJson(c.connector(common.StateInstalled))); got != c.body {
				t.Errorf("rollback PUT body =\n%s\nwant\n%s", got, c.body)
			}
		})
	}
}

// Reading back what P0 answers gives the model that was sent, plus what P0 fills in:
// the state, the connector's identity from the response's metadata and, once a Cloud
// Run connector is verified, its URL.
func TestConnectorFromJson(t *testing.T) {
	for _, c := range connectorClouds {
		for _, state := range []string{common.StateStage, common.StateConfigure, common.StateInstalled} {
			t.Run(c.name+" at "+state, func(t *testing.T) {
				var diags diag.Diagnostics
				api := getConnectorItemJson(connectorApiOf(t, c.response(state)))

				got, ok := (&ClickHouseConnector{}).fromJson(context.Background(), &diags, "prod", api).(*clickhouseConnectorModel)
				if !ok || diags.HasError() {
					t.Fatalf("fromJson failed: %v", diags)
				}
				if want := c.connector(state); !reflect.DeepEqual(got, want) {
					t.Errorf("fromJson =\n%+v\nwant\n%+v", got, want)
				}

				// The staged resource reads the same item, and never has the URL.
				staged, ok := (&ClickHouseConnectorStaged{}).fromJson(context.Background(), &diags, "prod", api).(*clickhouseConnectorStagedModel)
				if !ok || diags.HasError() {
					t.Fatalf("staged fromJson failed: %v", diags)
				}
				if want := c.staged(state); !reflect.DeepEqual(staged, want) {
					t.Errorf("staged fromJson =\n%+v\nwant\n%+v", staged, want)
				}
			})
		}
	}
}

// The model that fromJson returns is one Terraform's state takes.
func TestConnectorModelsFitState(t *testing.T) {
	for _, c := range connectorClouds {
		rawOf(t, NewClickHouseConnector(), c.connector(common.StateInstalled))
		rawOf(t, NewClickHouseConnectorStaged(), c.staged(common.StateStage))
	}
}

// A response without metadata leaves the identity null rather than failing.
func TestConnectorFromJsonWithoutMetadata(t *testing.T) {
	api := connectorApiOf(t, `{"item":{"hosting":{"type":"aws","accountId":"123456789012","connectorName":"p0-clickhouse-prod","connectorRegion":"us-west-2"},"state":"stage"}}`)

	var diags diag.Diagnostics
	got, ok := (&ClickHouseConnector{}).fromJson(context.Background(), &diags, "prod", getConnectorItemJson(api)).(*clickhouseConnectorModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson failed: %v", diags)
	}
	if !got.ConnectorIdentity.IsNull() {
		t.Errorf("connector identity = %v; want null", got.ConnectorIdentity)
	}
}

func TestConnectorFromJsonRequiresHosting(t *testing.T) {
	api := connectorApiOf(t, `{"item":{"state":"stage"}}`)
	cases := map[string]func(*diag.Diagnostics) any{
		"connector": func(diags *diag.Diagnostics) any {
			return (&ClickHouseConnector{}).fromJson(context.Background(), diags, "prod", api)
		},
		"connector staged": func(diags *diag.Diagnostics) any {
			return (&ClickHouseConnectorStaged{}).fromJson(context.Background(), diags, "prod", api)
		},
	}

	for name, fromJson := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := fromJson(&diags)
			if got != nil || len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != "Bad API response" {
				t.Errorf("fromJson = %+v, %v; want nil and a bad response error", got, diags)
			}
		})
	}
}

// A response with no item is a bad response, not an item to read.
func TestGetConnectorItemJsonRequiresItem(t *testing.T) {
	if got := getConnectorItemJson(connectorApiOf(t, `{"ok":true}`)); got != nil {
		t.Errorf("getConnectorItemJson = %+v; want nil", got)
	}
}

// P0 fixes where a connector runs when it creates the item, so changing the ID or any
// part of the hosting replaces the connector, in both resources.
func TestConnectorSchemaReplacement(t *testing.T) {
	for name, r := range map[string]resource.Resource{"connector staged": NewClickHouseConnectorStaged(), "connector": NewClickHouseConnector()} {
		s := schemaOf(t, r)
		hosting := nestedAttributes(t, s, "hosting")
		for field, attribute := range map[string]schema.Attribute{
			"id":                       s.Attributes["id"],
			"hosting.type":             hosting["type"],
			"hosting.account_id":       hosting["account_id"],
			"hosting.project_id":       hosting["project_id"],
			"hosting.connector_name":   hosting["connector_name"],
			"hosting.connector_region": hosting["connector_region"],
		} {
			if !requiresReplace(t, attribute) {
				t.Errorf("%s: %s doesn't require replacement", name, field)
			}
		}
	}
}

// What P0 assigns is read-only in both resources: the connector's identity, each of its
// fields, and the item's state.
func TestConnectorSchemaReadOnly(t *testing.T) {
	for name, r := range map[string]resource.Resource{"connector staged": NewClickHouseConnectorStaged(), "connector": NewClickHouseConnector()} {
		s := schemaOf(t, r)
		attributes := map[string]schema.Attribute{"connector_identity": s.Attributes["connector_identity"], "state": s.Attributes["state"]}
		for field, attribute := range nestedAttributes(t, s, "connector_identity") {
			attributes["connector_identity."+field] = attribute
		}
		for field, attribute := range attributes {
			if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
				t.Errorf("%s: %s = %+v; want a read-only attribute", name, field, attribute)
			}
		}
	}
}

// P0 resolves a Cloud Run service's URL when it verifies the install, so only the final
// resource's hosting has it. Without it, the final resource can take the staged
// resource's hosting whole.
func TestStagedHostingHasNoServiceUri(t *testing.T) {
	if _, ok := nestedAttributes(t, schemaOf(t, NewClickHouseConnectorStaged()), "hosting")["connector_service_uri"]; ok {
		t.Errorf("the staged connector's hosting has connector_service_uri")
	}

	uri := nestedAttributes(t, schemaOf(t, NewClickHouseConnector()), "hosting")["connector_service_uri"]
	if uri == nil || !uri.IsComputed() || uri.IsOptional() || uri.IsRequired() {
		t.Errorf("the connector's hosting.connector_service_uri = %+v; want a read-only attribute", uri)
	}
}

// P0's messages, as its own tests spell them out.
const (
	awsNameRule    = "Use 2 to 64 characters for the connector name: lowercase letters, digits and single hyphens, starting with a letter and ending with a letter or digit."
	awsAccountRule = "Enter the 12-digit ID of the AWS account the connector runs in."
	awsRegionRule  = "Enter the AWS region the connector runs in, such as us-west-2."
	gcpNameRule    = "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, starting with a letter and ending with a letter or digit. The connector's service account takes the same name."
	gcpProjectRule = "Enter the ID of the Google Cloud project the connector runs in."
	gcpRegionRule  = "Enter the Google Cloud region the connector runs in, such as us-west1."
	unsupported    = "P0's ClickHouse Cloud integration supports only AWS's commercial regions."
)

var (
	awsHostingFields = map[string]tftypes.Value{"type": str("aws"), "account_id": str("123456789012"), "connector_name": str("p0-clickhouse-prod"), "connector_region": str("us-west-2")}
	gcpHostingFields = map[string]tftypes.Value{"type": str("gcp"), "project_id": str("acme-prod"), "connector_name": str("p0-clickhouse-prod"), "connector_region": str("us-west1")}
)

// P0's own cases for its hosting rules
// (packages/integrations/clickhouse/backend/src/install/__tests__/connector.test.ts in
// the app), plus the edges of each rule and the fields each type takes, run through
// both resources' ValidateConfig: each refused value gets one error, at its attribute,
// with P0's message. Terraform calls ValidateConfig while planning, before the provider
// calls P0, and each check skips a value Terraform doesn't know yet.
func TestConnectorValidateConfig(t *testing.T) {
	aws := func(field, value string) func(tftypes.Object) tftypes.Value {
		return with(awsHostingFields, field, str(value))
	}
	gcp := func(field, value string) func(tftypes.Object) tftypes.Value {
		return with(gcpHostingFields, field, str(value))
	}

	cases := []struct {
		name    string
		hosting func(tftypes.Object) tftypes.Value
		want    []string
	}{
		{name: "aws", hosting: objectOf(awsHostingFields)},
		{name: "gcp", hosting: objectOf(gcpHostingFields)},
		{name: "unknown hosting", hosting: unknownObject},
		{name: "unknown hosting type", hosting: objectOf(map[string]tftypes.Value{"type": unknownString, "connector_name": str("P0")})},
		{name: "unknown connector name", hosting: with(awsHostingFields, "connector_name", unknownString)},
		{name: "unknown account", hosting: with(awsHostingFields, "account_id", unknownString)},
		{name: "unknown project", hosting: with(gcpHostingFields, "project_id", unknownString)},

		// AWS: a Lambda function, its ECR repository and its IAM role.
		{name: "a longest Lambda name", hosting: aws("connector_name", "p"+strings.Repeat("0", 63))},
		{name: "a shortest Lambda name", hosting: aws("connector_name", "p0")},
		{name: "a name longer than a role's", hosting: aws("connector_name", "p"+strings.Repeat("0", 64)), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name ECR wouldn't take", hosting: aws("connector_name", "p0--clickhouse"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name of one character", hosting: aws("connector_name", "p"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with capitals", hosting: aws("connector_name", "P0-ClickHouse"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name with an underscore", hosting: aws("connector_name", "p0_clickhouse"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "a name that starts with a digit", hosting: aws("connector_name", "0p-clickhouse"), want: []string{"hosting.connector_name: " + awsNameRule}},
		{name: "an account that isn't an AWS account ID", hosting: aws("account_id", "acme-prod"), want: []string{"hosting.account_id: " + awsAccountRule}},
		{name: "an 11-digit account", hosting: aws("account_id", "11111111111"), want: []string{"hosting.account_id: " + awsAccountRule}},
		{name: "a 13-digit account", hosting: aws("account_id", "1111111111111"), want: []string{"hosting.account_id: " + awsAccountRule}},
		{name: "a Google Cloud region on AWS", hosting: aws("connector_region", "us-west1"), want: []string{"hosting.connector_region: " + awsRegionRule}},
		{name: "an availability zone", hosting: aws("connector_region", "us-west-2a"), want: []string{"hosting.connector_region: " + awsRegionRule}},
		// P0 checks that a value is a region before it checks the region's partition.
		{name: "half a GovCloud region", hosting: aws("connector_region", "us-gov-west"), want: []string{"hosting.connector_region: " + awsRegionRule}},
		{name: "GovCloud", hosting: aws("connector_region", "us-gov-west-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "China", hosting: aws("connector_region", "cn-north-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ISO", hosting: aws("connector_region", "us-iso-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ISO-B", hosting: aws("connector_region", "us-isob-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "the European Sovereign Cloud", hosting: aws("connector_region", "eusc-de-east-1"), want: []string{"hosting.connector_region: " + unsupported}},
		{name: "ap-southeast-7", hosting: aws("connector_region", "ap-southeast-7")},

		// Google Cloud: a Cloud Run service and its service account.
		{name: "a longest service account name", hosting: gcp("connector_name", "p0-clickhouse-"+strings.Repeat("x", 16))},
		{name: "a shortest service account name", hosting: gcp("connector_name", "p0-doc")},
		{name: "a name too long for a service account", hosting: gcp("connector_name", "p0-clickhouse-"+strings.Repeat("x", 17)), want: []string{"hosting.connector_name: " + gcpNameRule}},
		{name: "a name too short for a service account", hosting: gcp("connector_name", "p0-dh"), want: []string{"hosting.connector_name: " + gcpNameRule}},
		{name: "a service account name with a trailing hyphen", hosting: gcp("connector_name", "p0-clickhouse-"), want: []string{"hosting.connector_name: " + gcpNameRule}},
		{name: "a project that isn't a project ID", hosting: gcp("project_id", "111111111111"), want: []string{"hosting.project_id: " + gcpProjectRule}},
		{name: "a longest project ID", hosting: gcp("project_id", "acme-"+strings.Repeat("x", 25))},
		{name: "a shortest project ID", hosting: gcp("project_id", "acme-1")},
		{name: "a project ID too long", hosting: gcp("project_id", "acme-"+strings.Repeat("x", 26)), want: []string{"hosting.project_id: " + gcpProjectRule}},
		{name: "a project ID too short", hosting: gcp("project_id", "acme1"), want: []string{"hosting.project_id: " + gcpProjectRule}},
		{name: "an AWS region on Google Cloud", hosting: gcp("connector_region", "us-west-2"), want: []string{"hosting.connector_region: " + gcpRegionRule}},
		{name: "a Google Cloud zone", hosting: gcp("connector_region", "us-west1-a"), want: []string{"hosting.connector_region: " + gcpRegionRule}},
		{name: "northamerica-northeast1", hosting: gcp("connector_region", "northamerica-northeast1")},
		{name: "europe-west12", hosting: gcp("connector_region", "europe-west12")},

		// The address fields each type takes.
		{name: "aws without an account", hosting: without(awsHostingFields, "account_id"), want: []string{`hosting.account_id: 'hosting.account_id' is required when 'hosting.type' is "aws".`}},
		{
			name:    "aws with a project",
			hosting: with(awsHostingFields, "project_id", str("acme-prod")),
			want:    []string{`hosting.project_id: 'hosting.project_id' may only be set when 'hosting.type' is "gcp".`},
		},
		{name: "gcp without a project", hosting: without(gcpHostingFields, "project_id"), want: []string{`hosting.project_id: 'hosting.project_id' is required when 'hosting.type' is "gcp".`}},
		{
			name:    "gcp with an account",
			hosting: with(gcpHostingFields, "account_id", str("123456789012")),
			want:    []string{`hosting.account_id: 'hosting.account_id' may only be set when 'hosting.type' is "aws".`},
		},

		// P0 checks a value trimmed, and stores it trimmed, so a padded value that's valid
		// otherwise is refused for its whitespace alone.
		{
			name:    "a padded name",
			hosting: aws("connector_name", " p0-clickhouse-prod "),
			want: []string{"hosting.connector_name: P0 removes the whitespace around 'hosting.connector_name', so the value it stores " +
				"wouldn't match this configuration. Remove the whitespace, for example with trimspace()."},
		},
		{name: "a padded region in China", hosting: aws("connector_region", " cn-north-1"), want: []string{"hosting.connector_region: " + unsupported}},
	}

	resources := map[string]resource.ResourceWithValidateConfig{"connector staged": &ClickHouseConnectorStaged{}, "connector": &ClickHouseConnector{}}
	for _, c := range cases {
		for name, r := range resources {
			t.Run(c.name+", "+name, func(t *testing.T) {
				blocks := map[string]func(tftypes.Object) tftypes.Value{"hosting": c.hosting}
				got := errorDetails(validateConfig(t, r, blocks, map[string]tftypes.Value{"id": str("prod")}))
				want := append([]string{}, c.want...)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("errors =\n%v\nwant\n%v", got, want)
				}
			})
		}
	}
}

// Terraform shows a nested attribute's error at its resource, so each summary says what
// the value is for.
func TestConnectorValidateConfigSummaries(t *testing.T) {
	cases := []struct {
		name    string
		hosting func(tftypes.Object) tftypes.Value
		want    []string
	}{
		{name: "aws name", hosting: with(awsHostingFields, "connector_name", str("P0")), want: []string{"Invalid connector name"}},
		{name: "aws account", hosting: with(awsHostingFields, "account_id", str("acme")), want: []string{"Invalid AWS account ID for the connector"}},
		{name: "aws region", hosting: with(awsHostingFields, "connector_region", str("us-west")), want: []string{"Invalid AWS region for the connector"}},
		{name: "aws partition", hosting: with(awsHostingFields, "connector_region", str("us-gov-west-1")), want: []string{"Unsupported AWS region for the connector"}},
		{name: "gcp name", hosting: with(gcpHostingFields, "connector_name", str("P0")), want: []string{"Invalid connector name"}},
		{name: "gcp project", hosting: with(gcpHostingFields, "project_id", str("acme")), want: []string{"Invalid Google Cloud project for the connector"}},
		{name: "gcp region", hosting: with(gcpHostingFields, "connector_region", str("us-west")), want: []string{"Invalid Google Cloud region for the connector"}},
		{name: "aws without an account", hosting: without(awsHostingFields, "account_id"), want: []string{"Missing AWS account ID"}},
		{name: "aws with a project", hosting: with(awsHostingFields, "project_id", str("acme-prod")), want: []string{"Unexpected Google Cloud project"}},
		{name: "gcp without a project", hosting: without(gcpHostingFields, "project_id"), want: []string{"Missing Google Cloud project"}},
		{name: "gcp with an account", hosting: with(gcpHostingFields, "account_id", str("123456789012")), want: []string{"Unexpected AWS account ID"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks := map[string]func(tftypes.Object) tftypes.Value{"hosting": c.hosting}
			got := errorSummaries(validateConfig(t, &ClickHouseConnector{}, blocks, map[string]tftypes.Value{"id": str("prod")}))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("errors = %v; want %v", got, c.want)
			}
		})
	}
}

// ValidateConfig checks the hosting's address fields against P0's rules, so the
// attribute's own validators would only repeat its errors in other words.
func TestConnectorHostingFieldsHaveNoValidators(t *testing.T) {
	for _, name := range []string{"account_id", "project_id", "connector_name", "connector_region"} {
		field, ok := hostingAttribute("").Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("hosting.%s is not a string attribute", name)
		}
		if len(field.Validators) != 0 {
			t.Errorf("hosting.%s has %d validators; want none", name, len(field.Validators))
		}
	}
}

// Every field a rule names is one the block has. A misspelt name would read a null
// value, which skips the check silently.
func TestHostingRuleFieldNames(t *testing.T) {
	names := hostingFields(awsCloud.hosting(types.StringNull()))
	attributes := hostingAttribute("").Attributes
	for hostingType, fields := range hostingRules {
		for _, field := range fields {
			if _, ok := names[field.name]; !ok {
				t.Errorf("hosting rule for %s names %q, which the hosting doesn't have", hostingType, field.name)
			}
			if _, ok := attributes[field.name]; !ok {
				t.Errorf("hosting rule for %s names %q, which the attribute doesn't have", hostingType, field.name)
			}
		}
	}
}

// Staging a connector creates the item, and reads back what P0 names for it.
func TestConnectorStagedCreate(t *testing.T) {
	for _, c := range connectorClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			r := configure(t, &ClickHouseConnectorStaged{}, server)

			resp := create(t, r, rawOf(t, r, c.plannedStaged()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+basePath+" {}", "PUT "+connectorPath+" "+c.body)
			if want := rawOf(t, r, c.staged(common.StateStage)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// P0 refuses to add a connector twice, so one that exists is imported, not created,
// and a refused create saves nothing.
func TestConnectorStagedCreateRefusesAnItemThatExists(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, connectorPath, common.StateInstalled, json.RawMessage(awsCloud.body))
	r := configure(t, &ClickHouseConnectorStaged{}, server)

	resp := create(t, r, rawOf(t, r, awsCloud.plannedStaged()))

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not stage connector component" {
		t.Fatalf("diagnostics = %v; want the refused stage", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v; want none", resp.State.Raw)
	}
	if got := fake.State(connectorPath); got != common.StateInstalled {
		t.Errorf("P0 item state = %q; want it left installed", got)
	}
}

// Every configurable attribute requires replacement, so Terraform never plans an
// update. If it did, staging the item again would be refused, so the update sends
// nothing and fails.
func TestConnectorStagedUpdateSendsNothing(t *testing.T) {
	fake, server := newFakeItems(t)
	r := configure(t, &ClickHouseConnectorStaged{}, server)
	prior := rawOf(t, r, awsCloud.staged(common.StateStage))

	resp := update(t, r, prior, rawOf(t, r, awsCloud.plannedStaged()))

	if !resp.Diagnostics.HasError() {
		t.Errorf("Update succeeded; want it refused")
	}
	wantRequests(t, fake)
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state = %v; want it unchanged", resp.State.Raw)
	}
}

func TestConnectorStagedDelete(t *testing.T) {
	fake, server := newFakeItems(t)
	fake.seed(t, connectorPath, common.StateStage, json.RawMessage(awsCloud.body))
	r := configure(t, &ClickHouseConnectorStaged{}, server)

	resp := destroy(t, r, rawOf(t, r, awsCloud.staged(common.StateStage)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+connectorPath)
	if got := fake.State(connectorPath); got != "" {
		t.Errorf("P0 item state = %q; want the item removed", got)
	}
}

// The staged resource created the item, and P0 refuses a second PUT of it, so the final
// resource only verifies and configures. On Google Cloud that resolves the URL.
func TestConnectorCreate(t *testing.T) {
	for _, c := range connectorClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, connectorPath, common.StateStage, json.RawMessage(c.body))
			r := configure(t, &ClickHouseConnector{}, server)

			resp := create(t, r, rawOf(t, r, c.plannedConnector()))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Create: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+connectorPath+"/verify "+c.body, "POST "+connectorPath+"/configure "+c.body)
			if want := rawOf(t, r, c.connector(common.StateInstalled)); !resp.State.Raw.Equal(want) {
				t.Errorf("state =\n%v\nwant\n%v", resp.State.Raw, want)
			}
		})
	}
}

// A failed check, or a connector that was never staged, fails the apply and saves
// nothing, so the next apply creates the resource again.
func TestConnectorCreateFailures(t *testing.T) {
	cases := []struct {
		name       string
		staged     bool
		failVerify bool
		want       string
	}{
		{name: "failed check", staged: true, failVerify: true, want: "ClickHouse Cloud connector install check failed"},
		{name: "never staged", want: "Could not verify connector component"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.failVerify = c.failVerify
			if c.staged {
				fake.seed(t, connectorPath, common.StateStage, json.RawMessage(awsCloud.body))
			}
			r := configure(t, &ClickHouseConnector{}, server)

			resp := create(t, r, rawOf(t, r, awsCloud.plannedConnector()))

			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 || errs[0].Summary() != c.want {
				t.Fatalf("diagnostics = %v; want %q", resp.Diagnostics, c.want)
			}
			wantRequests(t, fake, "POST "+connectorPath+"/verify "+awsCloud.body)
			if !resp.State.Raw.IsNull() {
				t.Errorf("state = %v; want none", resp.State.Raw)
			}
		})
	}
}

// Destroying the final resource returns the item to "stage", which the staged resource
// still owns. P0 refuses a PUT of an item that exists, so the item is removed and then
// staged again, where it was.
func TestConnectorDeleteStagesAgain(t *testing.T) {
	for _, c := range connectorClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, connectorPath, common.StateInstalled, json.RawMessage(c.body))
			r := configure(t, &ClickHouseConnector{}, server)

			resp := destroy(t, r, rawOf(t, r, c.connector(common.StateInstalled)))

			if resp.Diagnostics.HasError() {
				t.Fatalf("Delete: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "DELETE "+connectorPath, "PUT "+connectorPath+" "+c.body)
			if got := fake.State(connectorPath); got != common.StateStage {
				t.Errorf("P0 item state = %q; want stage", got)
			}
		})
	}
}

// A destroy that removed the item but failed to stage it again is retried from an item
// that's gone, and still ends with the item staged.
func TestConnectorDeleteWhenTheItemIsGone(t *testing.T) {
	fake, server := newFakeItems(t)
	r := configure(t, &ClickHouseConnector{}, server)

	resp := destroy(t, r, rawOf(t, r, awsCloud.connector(common.StateInstalled)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics)
	}
	wantRequests(t, fake, "DELETE "+connectorPath, "PUT "+connectorPath+" "+awsCloud.body)
	if got := fake.State(connectorPath); got != common.StateStage {
		t.Errorf("P0 item state = %q; want stage", got)
	}
}

// A connector that P0 hasn't installed plans an update even when its configuration
// hasn't changed, so that the next apply finishes the install. Its state, and the URL
// that P0 resolves when it verifies, are unknown in it. Every other plan is the
// framework's, so an installed connector plans no difference.
func TestConnectorModifyPlan(t *testing.T) {
	r := &ClickHouseConnector{}
	for _, c := range connectorClouds {
		at := func(state string) tftypes.Value { return rawOf(t, r, c.connector(state)) }
		// The plan that finishes the install of a connector at state.
		finishing := func(state string) tftypes.Value {
			model := c.connector(state)
			model.Hosting.ConnectorServiceUri = types.StringUnknown()
			model.State = types.StringUnknown()
			return rawOf(t, r, model)
		}
		none := rawOf(t, r, (*clickhouseConnectorModel)(nil))

		cases := []struct {
			name  string
			prior tftypes.Value // The refreshed state, null on create.
			plan  tftypes.Value // The framework's plan, null on destroy.
			want  tftypes.Value
		}{
			{name: "installed", prior: at(common.StateInstalled), plan: at(common.StateInstalled), want: at(common.StateInstalled)},
			{name: "configure", prior: at(common.StateConfigure), plan: at(common.StateConfigure), want: finishing(common.StateConfigure)},
			{name: "stage", prior: at(common.StateStage), plan: at(common.StateStage), want: finishing(common.StateStage)},
			{name: "create", prior: none, plan: rawOf(t, r, c.plannedConnector()), want: rawOf(t, r, c.plannedConnector())},
			{name: "destroy", prior: at(common.StateInstalled), plan: none, want: none},
		}

		for _, tc := range cases {
			t.Run(c.name+" "+tc.name, func(t *testing.T) {
				resp := modifyPlan(t, r, tc.prior, tc.plan)
				if resp.Diagnostics.HasError() {
					t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
				}
				if !resp.Plan.Raw.Equal(tc.want) {
					t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, tc.want)
				}
			})
		}
	}
}

// A hosting block that Terraform only knows at apply time stays unknown: a known block
// of unknown fields in its place wouldn't match the configuration.
func TestConnectorModifyPlanUnknownHosting(t *testing.T) {
	ctx := context.Background()
	r := &ClickHouseConnector{}
	s := schemaOf(t, r)
	hostingType, ok := s.Attributes["hosting"].GetType().(types.ObjectType)
	if !ok {
		t.Fatalf("hosting is not an object")
	}
	withUnknownHosting := func(state types.String) tftypes.Value {
		model := awsCloud.connector(common.StateStage)
		model.State = state
		plan := tfsdk.Plan{Schema: s, Raw: rawOf(t, r, model)}
		if diags := plan.SetAttribute(ctx, path.Root("hosting"), types.ObjectUnknown(hostingType.AttrTypes)); diags.HasError() {
			t.Fatalf("SetAttribute: %v", diags)
		}
		return plan.Raw
	}

	prior := rawOf(t, r, awsCloud.connector(common.StateStage))
	resp := modifyPlan(t, r, prior, withUnknownHosting(types.StringValue(common.StateStage)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
	}
	if want := withUnknownHosting(types.StringUnknown()); !resp.Plan.Raw.Equal(want) {
		t.Errorf("plan =\n%v\nwant\n%v", resp.Plan.Raw, want)
	}
}

// A connector imported at stage, or staged again outside this resource, converges in
// one apply: its plan shows an update, the update verifies and configures it without
// staging it, and the plan after that shows no difference.
func TestConnectorStagedItemConverges(t *testing.T) {
	for _, c := range connectorClouds {
		t.Run(c.name, func(t *testing.T) {
			fake, server := newFakeItems(t)
			fake.seed(t, connectorPath, common.StateStage, json.RawMessage(c.body))
			r := configure(t, &ClickHouseConnector{}, server)

			staged := rawOf(t, r, c.connector(common.StateStage))
			plan := modifyPlan(t, r, staged, staged)
			if plan.Diagnostics.HasError() || plan.Plan.Raw.Equal(staged) {
				t.Fatalf("plan = %v, %v; want an update", plan.Plan.Raw, plan.Diagnostics)
			}

			resp := update(t, r, staged, plan.Plan.Raw)

			if resp.Diagnostics.HasError() {
				t.Fatalf("Update: %v", resp.Diagnostics)
			}
			wantRequests(t, fake, "POST "+connectorPath+"/verify "+c.body, "POST "+connectorPath+"/configure "+c.body)
			installed := rawOf(t, r, c.connector(common.StateInstalled))
			if !resp.State.Raw.Equal(installed) {
				t.Fatalf("state =\n%v\nwant the installed connector", resp.State.Raw)
			}
			if replan := modifyPlan(t, r, installed, installed); replan.Diagnostics.HasError() || !replan.Plan.Raw.Equal(installed) {
				t.Errorf("plan after the update = %v, %v; want no difference", replan.Plan.Raw, replan.Diagnostics)
			}
		})
	}
}
