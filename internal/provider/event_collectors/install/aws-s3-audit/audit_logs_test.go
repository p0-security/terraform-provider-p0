package installawss3audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Decodes a state with every schema attribute into the model, as the framework
// does on plan and apply.
func decodeNullState(t *testing.T, r resource.Resource, model any) {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(objectType, values),
	}
	if diags := state.Get(ctx, model); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
}

func TestAuditLogsStagedModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewAuditLogsStaged(), &auditLogsStagedModel{})
}

func TestAuditLogsModelMatchesSchema(t *testing.T) {
	decodeNullState(t, NewAuditLogs(), &auditLogsModel{})
}

func TestBucketNameRegex(t *testing.T) {
	for name, want := range map[string]bool{
		"p0-audit-logs":       true,
		"p0.audit.logs":       true,
		"abc":                 true,
		"ab":                  false,
		"P0-Audit":            false,
		"-p0-audit":           false,
		"p0_audit":            false,
		"s3://p0-audit-logs":  false,
		"arn:aws:s3:::bucket": false,
	} {
		if got := bucketNameRegex.MatchString(name); got != want {
			t.Errorf("bucketNameRegex(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestRegionRegex(t *testing.T) {
	for region, want := range map[string]bool{
		"us-west-2":     true,
		"eu-central-1":  true,
		"us-gov-west-1": true,
		"us-west":       false,
		"US-WEST-2":     false,
		"us-west-2a":    false,
	} {
		if got := regionRegex.MatchString(region); got != want {
			t.Errorf("regionRegex(%q) = %v, want %v", region, got, want)
		}
	}
}

// P0 reads the account and region from the item to find the AWS integration's
// service account, and stores the partition as {"type": ...}.
func TestBucketFieldsJson(t *testing.T) {
	fields := bucketFields{
		AccountId: types.StringValue("123456789012"),
		Region:    types.StringValue("us-west-2"),
		Partition: types.StringValue("aws-us-gov"),
	}
	got, err := json.Marshal(fields.toJson())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"accountId":"123456789012","region":"us-west-2","awsPartition":{"type":"aws-us-gov"}}`
	if string(got) != want {
		t.Errorf("toJson = %s, want %s", got, want)
	}

	var item auditLogsJson
	if err := json.Unmarshal(got, &item); err != nil {
		t.Fatal(err)
	}
	if back := bucketFieldsFromJson(&item); back != fields {
		t.Errorf("bucketFieldsFromJson = %+v, want %+v", back, fields)
	}
}

func TestBucketFieldsDefaultPartition(t *testing.T) {
	if got := bucketFieldsFromJson(&auditLogsJson{}).Partition.ValueString(); got != "aws" {
		t.Errorf("partition = %q, want aws", got)
	}
}
