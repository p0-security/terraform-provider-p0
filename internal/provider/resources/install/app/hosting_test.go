package installapp

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestConnectorHostingFromJson(t *testing.T) {
	cases := []struct {
		name string
		json string
		want ConnectorHostingModel
	}{
		{
			name: "gcp hosting with the fields P0 generates",
			json: `{
				"type": "gcp",
				"projectId": "my-project-id",
				"connectorName": "p0-connector",
				"connectorRegion": "us-central1",
				"connectorServiceAccount": "p0-connector@my-project-id.iam.gserviceaccount.com",
				"connectorServiceUri": "https://p0-connector-abc123-uc.a.run.app"
			}`,
			want: ConnectorHostingModel{
				Type:                    types.StringValue(GcpHosting),
				AccountId:               types.StringNull(),
				ProjectId:               types.StringValue("my-project-id"),
				ConnectorName:           types.StringValue("p0-connector"),
				ConnectorRegion:         types.StringValue("us-central1"),
				ConnectorServiceAccount: types.StringValue("p0-connector@my-project-id.iam.gserviceaccount.com"),
				ConnectorServiceUri:     types.StringValue("https://p0-connector-abc123-uc.a.run.app"),
			},
		},
		{
			// P0 doesn't generate a service account for a custom application, and an
			// install made before it generated one has none either.
			name: "gcp hosting without a service account",
			json: `{
				"type": "gcp",
				"projectId": "my-project-id",
				"connectorName": "p0-connector",
				"connectorRegion": "us-central1"
			}`,
			want: ConnectorHostingModel{
				Type:                    types.StringValue(GcpHosting),
				AccountId:               types.StringNull(),
				ProjectId:               types.StringValue("my-project-id"),
				ConnectorName:           types.StringValue("p0-connector"),
				ConnectorRegion:         types.StringValue("us-central1"),
				ConnectorServiceAccount: types.StringNull(),
				ConnectorServiceUri:     types.StringNull(),
			},
		},
		{
			name: "aws hosting",
			json: `{
				"type": "aws",
				"accountId": "123456789012",
				"connectorName": "p0-connector",
				"connectorRegion": "us-east-1"
			}`,
			want: ConnectorHostingModel{
				Type:                    types.StringValue(AwsHosting),
				AccountId:               types.StringValue("123456789012"),
				ProjectId:               types.StringNull(),
				ConnectorName:           types.StringValue("p0-connector"),
				ConnectorRegion:         types.StringValue("us-east-1"),
				ConnectorServiceAccount: types.StringNull(),
				ConnectorServiceUri:     types.StringNull(),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hosting ConnectorHostingJson
			if err := json.Unmarshal([]byte(c.json), &hosting); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := *ConnectorHostingFromJson(&hosting); got != c.want {
				t.Errorf("ConnectorHostingFromJson() = %+v; want %+v", got, c.want)
			}
		})
	}
}

// P0 generates the service account and resolves the URI, so the provider never sends
// either back, even once it has read them.
func TestConnectorHostingToJsonOmitsGeneratedFields(t *testing.T) {
	model := ConnectorHostingModel{
		Type:                    types.StringValue(GcpHosting),
		AccountId:               types.StringNull(),
		ProjectId:               types.StringValue("my-project-id"),
		ConnectorName:           types.StringValue("p0-connector"),
		ConnectorRegion:         types.StringValue("us-central1"),
		ConnectorServiceAccount: types.StringValue("p0-connector@my-project-id.iam.gserviceaccount.com"),
		ConnectorServiceUri:     types.StringValue("https://p0-connector-abc123-uc.a.run.app"),
	}

	got, err := json.Marshal(model.ToJson())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"type":"gcp","projectId":"my-project-id","connectorName":"p0-connector","connectorRegion":"us-central1"}`
	if string(got) != want {
		t.Errorf("ToJson() = %s; want %s", got, want)
	}
}
