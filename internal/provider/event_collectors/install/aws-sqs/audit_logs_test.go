package installawssqs

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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

func TestQueueArnRegex(t *testing.T) {
	for arn, want := range map[string]bool{
		"arn:aws:sqs:us-east-1:123456789012:p0-audit":            true,
		"arn:aws:sqs:us-west-2:123456789012:p0-audit.fifo":       true,
		"arn:aws-us-gov:sqs:us-gov-west-1:123456789012:p0-audit": true,
		"https://sqs.us-east-1.amazonaws.com/123456789012/queue": false,
		"arn:aws:sns:us-east-1:123456789012:p0-audit":            false,
		"arn:aws:sqs:us-east-1:1234:p0-audit":                    false,
	} {
		if got := queueArnRegex.MatchString(arn); got != want {
			t.Errorf("queueArnRegex(%q) = %v, want %v", arn, got, want)
		}
	}
}

func TestKmsKeyArnRegex(t *testing.T) {
	for arn, want := range map[string]bool{
		"arn:aws:kms:us-east-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab": true,
		"arn:aws:kms:us-east-1:123456789012:alias/aws/sqs":                            false,
	} {
		if got := kmsKeyArnRegex.MatchString(arn); got != want {
			t.Errorf("kmsKeyArnRegex(%q) = %v, want %v", arn, got, want)
		}
	}
}
