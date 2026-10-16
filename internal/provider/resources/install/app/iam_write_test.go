package installapp

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Runs p0_app's ValidateConfig against a configuration whose hosting is the given value.
func validateAppConfig(t *testing.T, hosting func(tftypes.Object) tftypes.Value) []string {
	t.Helper()
	ctx := context.Background()
	r := &appIamWrite{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	hostingType, ok := objectType.AttributeTypes["hosting"].(tftypes.Object)
	if !ok {
		t.Fatalf("hosting type is not an object")
	}

	config := tftypes.NewValue(objectType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "internal-admin-tool"),
		"hosting": hosting(hostingType),
		"label":   tftypes.NewValue(tftypes.String, nil),
		"state":   tftypes.NewValue(tftypes.String, nil),
	})

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)
	summaries := []string{}
	for _, d := range resp.Diagnostics.Errors() {
		summaries = append(summaries, d.Summary())
	}
	return summaries
}

// A hosting block with the given fields set and every other field null.
func hostingValue(fields map[string]tftypes.Value) func(tftypes.Object) tftypes.Value {
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

const (
	testDirectoryId    = "11111111-2222-3333-4444-555555555555"
	testSubscriptionId = "66666666-7777-8888-9999-000000000000"
)

// A valid Azure hosting block, with the given fields changed.
func azureHostingValue(changes map[string]tftypes.Value) func(tftypes.Object) tftypes.Value {
	str := func(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }
	fields := map[string]tftypes.Value{
		"type":             str(AzureHosting),
		"directory_id":     str(testDirectoryId),
		"subscription_id":  str(testSubscriptionId),
		"resource_group":   str("p0-connectors"),
		"connector_name":   str("p0-connector"),
		"connector_region": str("eastus"),
	}
	for name, value := range changes {
		fields[name] = value
	}
	return hostingValue(fields)
}

// Terraform validates a configuration before it knows every value, so a hosting block
// set from a module output, a variable or for_each can be wholly unknown. Validation
// skips it rather than failing to read it.
func TestAppValidateConfigUnknownHosting(t *testing.T) {
	str := func(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }
	cases := []struct {
		name    string
		hosting func(tftypes.Object) tftypes.Value
		want    []string
	}{
		{
			name:    "unknown hosting",
			hosting: func(typ tftypes.Object) tftypes.Value { return tftypes.NewValue(typ, tftypes.UnknownValue) },
		},
		{
			name: "unknown hosting type",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"connector_name":   str("p0-connector"),
				"connector_region": str("us-east-1"),
			}),
		},
		{
			name: "aws hosting",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AwsHosting),
				"account_id":       str("123456789012"),
				"connector_name":   str("p0-connector"),
				"connector_region": str("us-east-1"),
			}),
		},
		{
			name: "aws hosting with a project",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AwsHosting),
				"project_id":       str("my-project-id"),
				"connector_name":   str("p0-connector"),
				"connector_region": str("us-east-1"),
			}),
			want: []string{"Missing AWS account ID", "Unexpected Google Cloud project"},
		},
		{
			name: "gcp hosting with a Lambda function's name",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(GcpHosting),
				"project_id":       str("my-project-id"),
				"connector_name":   str("P0_Connector"),
				"connector_region": str("us-central1"),
			}),
			want: []string{"Invalid Cloud Run service name"},
		},
		{
			name: "aws hosting with a Lambda function's name",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AwsHosting),
				"account_id":       str("123456789012"),
				"connector_name":   str("P0_Connector"),
				"connector_region": str("us-east-1"),
			}),
		},
		{
			name:    "azure hosting",
			hosting: azureHostingValue(nil),
		},
		{
			name: "azure hosting without its address",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AzureHosting),
				"connector_name":   str("p0-connector"),
				"connector_region": str("eastus"),
			}),
			want: []string{"Missing Azure directory ID", "Missing Azure subscription ID", "Missing Azure resource group"},
		},
		{
			name:    "azure hosting with an AWS account and a project",
			hosting: azureHostingValue(map[string]tftypes.Value{"account_id": str("123456789012"), "project_id": str("my-project-id")}),
			want:    []string{"Unexpected AWS account ID", "Unexpected Google Cloud project"},
		},
		{
			name: "aws hosting with an Azure address",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AwsHosting),
				"account_id":       str("123456789012"),
				"directory_id":     str(testDirectoryId),
				"subscription_id":  str(testSubscriptionId),
				"resource_group":   str("p0-connectors"),
				"connector_name":   str("p0-connector"),
				"connector_region": str("us-east-1"),
			}),
			want: []string{"Unexpected Azure directory ID", "Unexpected Azure subscription ID", "Unexpected Azure resource group"},
		},
		{
			name:    "azure hosting with a Lambda function's name",
			hosting: azureHostingValue(map[string]tftypes.Value{"connector_name": str("P0_Connector")}),
			want:    []string{"Invalid Container App name"},
		},
		{
			name:    "azure hosting with two hyphens in a row",
			hosting: azureHostingValue(map[string]tftypes.Value{"connector_name": str("p0--connector")}),
			want:    []string{"Invalid Container App name"},
		},
		{
			name:    "azure hosting with a name too long for a Container App",
			hosting: azureHostingValue(map[string]tftypes.Value{"connector_name": str("p0-connector-for-the-admin-tool-x")}),
			want:    []string{"Invalid Container App name"},
		},
		{
			name:    "azure hosting with an AWS region",
			hosting: azureHostingValue(map[string]tftypes.Value{"connector_region": str("us-east-1")}),
			want:    []string{"Invalid Azure region"},
		},
		{
			name: "aws hosting with an Azure region",
			hosting: hostingValue(map[string]tftypes.Value{
				"type":             str(AwsHosting),
				"account_id":       str("123456789012"),
				"connector_name":   str("p0-connector"),
				"connector_region": str("eastus"),
			}),
			want: []string{"Invalid region"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateAppConfig(t, c.hosting)
			if len(got) != len(c.want) {
				t.Fatalf("errors = %v; want %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("errors = %v; want %v", got, c.want)
				}
			}
		})
	}
}

// p0_app checks each address field with its own validators, which accept what either
// Lambda or Cloud Run does.
func TestAppHostingFieldValidators(t *testing.T) {
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	(&appIamWrite{}).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	hosting, ok := schemaResp.Schema.Attributes["hosting"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("hosting is not a nested attribute")
	}

	cases := []struct {
		field string
		value string
		want  bool
	}{
		{field: "account_id", value: "123456789012", want: true},
		{field: "account_id", value: "1234", want: false},
		{field: "project_id", value: "my-project-id", want: true},
		{field: "project_id", value: "my project", want: false},
		{field: "connector_name", value: "P0_Connector", want: true},
		{field: "connector_name", value: "1connector", want: false},
		{field: "connector_region", value: "us-east-1", want: true},
		{field: "connector_region", value: "us-central1", want: true},
		{field: "connector_region", value: "us-east", want: false},
		{field: "connector_region", value: "eastus", want: true},
		{field: "connector_region", value: "eastus2", want: true},
		{field: "connector_region", value: "East US", want: false},
		{field: "directory_id", value: testDirectoryId, want: true},
		{field: "directory_id", value: "contoso.onmicrosoft.com", want: false},
		{field: "subscription_id", value: testSubscriptionId, want: true},
		{field: "subscription_id", value: "my-subscription", want: false},
		{field: "resource_group", value: "p0-connectors", want: true},
		{field: "resource_group", value: "rg_(prod).eu", want: true},
		{field: "resource_group", value: "p0-connectors.", want: false},
		{field: "resource_group", value: "p0 connectors", want: false},
	}

	for _, c := range cases {
		t.Run(c.field+" "+c.value, func(t *testing.T) {
			attribute, ok := hosting.Attributes[c.field].(schema.StringAttribute)
			if !ok {
				t.Fatalf("hosting.%s is not a string attribute", c.field)
			}
			resp := &validator.StringResponse{}
			for _, v := range attribute.Validators {
				v.ValidateString(ctx, validator.StringRequest{
					Path:        path.Root("hosting").AtName(c.field),
					ConfigValue: types.StringValue(c.value),
				}, resp)
			}
			if got := !resp.Diagnostics.HasError(); got != c.want {
				t.Errorf("valid = %v; want %v (%v)", got, c.want, resp.Diagnostics)
			}
		})
	}
}
