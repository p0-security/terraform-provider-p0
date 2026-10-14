package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// The hosting block's tests sit here, not beside it, because CI runs only this
// package's tests.

const (
	hostingProject        = "my-project-id"
	hostingConnector      = "p0-connector"
	hostingRegion         = "us-central1"
	hostingServiceAccount = "p0-connector@my-project-id.iam.gserviceaccount.com"
	hostingServiceUri     = "https://p0-connector-abc123-uc.a.run.app"
)

// A Google Cloud hosting with the two fields P0 sets.
func gcpHostingModel(serviceAccount, serviceUri types.String) installapp.ConnectorHostingWithServiceAccountModel {
	return installapp.ConnectorHostingWithServiceAccountModel{
		ConnectorHostingModel: installapp.ConnectorHostingModel{
			Type:                types.StringValue(installapp.GcpHosting),
			AccountId:           types.StringNull(),
			ProjectId:           types.StringValue(hostingProject),
			ConnectorName:       types.StringValue(hostingConnector),
			ConnectorRegion:     types.StringValue(hostingRegion),
			ConnectorServiceUri: serviceUri,
		},
		ConnectorServiceAccount: serviceAccount,
	}
}

func TestConnectorHostingWithServiceAccountFromJson(t *testing.T) {
	cases := []struct {
		name string
		json string
		want installapp.ConnectorHostingWithServiceAccountModel
	}{
		{
			name: "gcp hosting with the fields P0 sets",
			json: `{
				"type": "gcp",
				"projectId": "` + hostingProject + `",
				"connectorName": "` + hostingConnector + `",
				"connectorRegion": "` + hostingRegion + `",
				"connectorServiceAccount": "` + hostingServiceAccount + `",
				"connectorServiceUri": "` + hostingServiceUri + `"
			}`,
			want: gcpHostingModel(types.StringValue(hostingServiceAccount), types.StringValue(hostingServiceUri)),
		},
		{
			name: "gcp hosting without a service account",
			json: `{
				"type": "gcp",
				"projectId": "` + hostingProject + `",
				"connectorName": "` + hostingConnector + `",
				"connectorRegion": "` + hostingRegion + `"
			}`,
			want: gcpHostingModel(types.StringNull(), types.StringNull()),
		},
		{
			name: "aws hosting",
			json: `{
				"type": "aws",
				"accountId": "123456789012",
				"connectorName": "` + hostingConnector + `",
				"connectorRegion": "us-east-1"
			}`,
			want: installapp.ConnectorHostingWithServiceAccountModel{
				ConnectorHostingModel: installapp.ConnectorHostingModel{
					Type:                types.StringValue(installapp.AwsHosting),
					AccountId:           types.StringValue("123456789012"),
					ProjectId:           types.StringNull(),
					ConnectorName:       types.StringValue(hostingConnector),
					ConnectorRegion:     types.StringValue("us-east-1"),
					ConnectorServiceUri: types.StringNull(),
				},
				ConnectorServiceAccount: types.StringNull(),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hosting installapp.ConnectorHostingJson
			if err := json.Unmarshal([]byte(c.json), &hosting); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := *installapp.ConnectorHostingWithServiceAccountFromJson(&hosting); got != c.want {
				t.Errorf("ConnectorHostingWithServiceAccountFromJson() = %+v; want %+v", got, c.want)
			}
		})
	}
}

// P0 sets the service account and the URI, so the provider never sends either back,
// even once it has read them.
func TestConnectorHostingToJsonOmitsGeneratedFields(t *testing.T) {
	model := gcpHostingModel(types.StringValue(hostingServiceAccount), types.StringValue(hostingServiceUri))

	encoded, err := json.Marshal(model.ToJson())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(encoded, &sent); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"connectorServiceAccount", "connectorServiceUri"} {
		if value, ok := sent[key]; ok {
			t.Errorf("ToJson() sends %s = %v; want it left out", key, value)
		}
	}
	// The address still goes out, so an empty body can't pass for one without the
	// generated fields.
	for key, want := range map[string]string{
		"type":            installapp.GcpHosting,
		"projectId":       hostingProject,
		"connectorName":   hostingConnector,
		"connectorRegion": hostingRegion,
	} {
		if sent[key] != want {
			t.Errorf("ToJson() sends %s = %v; want %q", key, sent[key], want)
		}
	}
}

// A schema whose only attribute is the hosting block, and its Terraform types.
func hostingSchema(t *testing.T, hosting schema.SingleNestedAttribute) (schema.Schema, tftypes.Object, tftypes.Object) {
	t.Helper()
	s := schema.Schema{Attributes: map[string]schema.Attribute{"hosting": hosting}}
	rootType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	hostingType, ok := rootType.AttributeTypes["hosting"].(tftypes.Object)
	if !ok {
		t.Fatalf("hosting type is not an object")
	}
	return s, rootType, hostingType
}

// Reading an object into a struct fails unless the two have the same fields, and no
// resource uses this block yet, so this is what holds the model to its attribute.
func TestConnectorHostingWithServiceAccountFromConfig(t *testing.T) {
	ctx := context.Background()
	s, rootType, hostingType := hostingSchema(t, installapp.ConnectorHostingWithServiceAccountAttribute(installapp.ConnectorHostingValidators{}))
	str := func(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }
	read := func(t *testing.T, hosting tftypes.Value) *installapp.ConnectorHostingWithServiceAccountModel {
		t.Helper()
		var diags diag.Diagnostics
		config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(rootType, map[string]tftypes.Value{"hosting": hosting})}
		got := installapp.ConnectorHostingWithServiceAccountFromConfig(ctx, config, &diags)
		if diags.HasError() {
			t.Fatalf("diagnostics: %v", diags)
		}
		return got
	}

	// A configuration never holds the two computed fields. They are set here so that
	// each of the model's fields is seen to read its own attribute.
	t.Run("every attribute set", func(t *testing.T) {
		got := read(t, tftypes.NewValue(hostingType, map[string]tftypes.Value{
			"type":                      str(installapp.GcpHosting),
			"account_id":                tftypes.NewValue(tftypes.String, nil),
			"project_id":                str(hostingProject),
			"connector_name":            str(hostingConnector),
			"connector_region":          str(hostingRegion),
			"connector_service_account": str(hostingServiceAccount),
			"connector_service_uri":     str(hostingServiceUri),
		}))
		want := gcpHostingModel(types.StringValue(hostingServiceAccount), types.StringValue(hostingServiceUri))
		if got == nil || *got != want {
			t.Errorf("ConnectorHostingWithServiceAccountFromConfig() = %+v; want %+v", got, want)
		}
	})

	t.Run("unknown hosting", func(t *testing.T) {
		if got := read(t, tftypes.NewValue(hostingType, tftypes.UnknownValue)); got != nil {
			t.Errorf("ConnectorHostingWithServiceAccountFromConfig() = %+v; want nil", got)
		}
	})
}

// p0_app uses the block without the service account, and must not gain the attribute:
// P0 generates none for a custom application, and an attribute can't be removed again
// once a release has it.
func TestAppHostingHasNoServiceAccount(t *testing.T) {
	with := installapp.ConnectorHostingWithServiceAccountAttribute(installapp.ConnectorHostingValidators{})
	if _, ok := with.Attributes["connector_service_account"]; !ok {
		t.Errorf("ConnectorHostingWithServiceAccountAttribute() has no connector_service_account")
	}

	var resp resource.SchemaResponse
	installapp.NewAppIamWrite().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	hosting, ok := resp.Schema.Attributes["hosting"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("p0_app's hosting is not a nested attribute")
	}
	if _, ok := hosting.Attributes["connector_service_account"]; ok {
		t.Errorf("p0_app's hosting has connector_service_account")
	}
}
