package installgcpcloudsql

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func strp(s string) *string { return &s }

func TestToJsonSendsOnlyProjectId(t *testing.T) {
	model := &gcpCloudSqlIamWriteModel{
		Id:                   types.StringValue("my-vpc"),
		ProjectId:            types.StringValue("my-project"),
		Region:               types.StringValue("us-west1"),
		ConnectorServiceName: types.StringValue("p0-db-my-vpc"),
	}

	out, ok := toJson(model).(*gcpCloudSqlIamWriteJson)
	if !ok {
		t.Fatal("toJson returned wrong type")
	}
	if out.ProjectId != "my-project" {
		t.Errorf("projectId = %q, want my-project", out.ProjectId)
	}
	if out.ConnectorRegion != nil {
		t.Errorf("connectorRegion = %v, want omitted", *out.ConnectorRegion)
	}
	if out.ConnectorServiceName != nil {
		t.Errorf("connectorServiceName = %v, want omitted", *out.ConnectorServiceName)
	}
}

func TestFromJsonMapsAllFields(t *testing.T) {
	jsonv := &gcpCloudSqlIamWriteJson{
		ProjectId:               "my-project",
		ConnectorRegion:         strp("us-west1"),
		ConnectorServiceName:    strp("p0-db-my-vpc"),
		ConnectorServiceUri:     strp("https://p0-db-my-vpc-abc.a.run.app"),
		ConnectorServiceAccount: strp("p0-db-my-vpc@my-project.iam.gserviceaccount.com"),
		State:                   "installed",
	}

	model, ok := fromJson(context.Background(), &diag.Diagnostics{}, "my-vpc", jsonv).(*gcpCloudSqlIamWriteModel)
	if !ok {
		t.Fatal("fromJson returned wrong type")
	}
	if model.Id.ValueString() != "my-vpc" {
		t.Errorf("id = %q, want my-vpc", model.Id.ValueString())
	}
	if model.ProjectId.ValueString() != "my-project" {
		t.Errorf("project_id = %q, want my-project", model.ProjectId.ValueString())
	}
	if model.Region.ValueString() != "us-west1" {
		t.Errorf("region = %q, want us-west1", model.Region.ValueString())
	}
	if model.ConnectorServiceName.ValueString() != "p0-db-my-vpc" {
		t.Errorf("connector_service_name = %q", model.ConnectorServiceName.ValueString())
	}
	if model.ConnectorServiceUri.ValueString() != "https://p0-db-my-vpc-abc.a.run.app" {
		t.Errorf("connector_service_uri = %q", model.ConnectorServiceUri.ValueString())
	}
	if model.ConnectorServiceAccount.ValueString() != "p0-db-my-vpc@my-project.iam.gserviceaccount.com" {
		t.Errorf("connector_service_account = %q", model.ConnectorServiceAccount.ValueString())
	}
	if model.State.ValueString() != "installed" {
		t.Errorf("state = %q, want installed", model.State.ValueString())
	}
}

func TestFromJsonNullConnectorUriWhenUnresolved(t *testing.T) {
	jsonv := &gcpCloudSqlIamWriteJson{
		ProjectId:            "my-project",
		ConnectorRegion:      strp("us-west1"),
		ConnectorServiceName: strp("p0-db-my-vpc"),
		State:                "stage",
	}

	model := fromJson(context.Background(), &diag.Diagnostics{}, "my-vpc", jsonv).(*gcpCloudSqlIamWriteModel)
	if !model.ConnectorServiceUri.IsNull() {
		t.Errorf("connector_service_uri = %q, want null", model.ConnectorServiceUri.ValueString())
	}
}
