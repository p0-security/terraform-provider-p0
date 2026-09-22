package installaigateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The state shapes below are what v0.53.0 wrote, under the resources' former
// names. They are deliberately not the current schema: v0.53.0's flat `url` and
// `oauth_endpoint` were replaced by the nested `domain_hosting` without a schema
// version bump, so a move must tolerate attributes the current schema no longer
// declares. Anything stricter would reject state a plain refresh accepts.
var moveTests = []struct {
	name       string
	newTarget  func() resource.Resource
	sourceType string
	priorState string
}{
	{
		name:       "gateway",
		newTarget:  NewGateway,
		sourceType: "p0_agentic_gateway",
		priorState: `{"id":"primary","url":"https://gateway.example.com",` +
			`"oauth_endpoint":"https://oauth.gateway.example.com",` +
			`"service_account_email":"p0@example.iam.gserviceaccount.com"}`,
	},
	{
		name:       "staged gateway",
		newTarget:  NewGatewayStaged,
		sourceType: "p0_agentic_gateway_staged",
		priorState: `{"id":"primary","url":"https://gateway.example.com",` +
			`"service_account_email":"p0@example.iam.gserviceaccount.com"}`,
	},
	{
		name:       "server",
		newTarget:  NewServer,
		sourceType: "p0_agentic_server",
		priorState: `{"id":"aws-tools","gateway":"primary"}`,
	},
}

// moveResponse mirrors how the framework pre-populates the response before
// calling a mover: the target's schema, and a null value of its type.
func moveResponse(ctx context.Context, t *testing.T, target resource.Resource) *resource.MoveStateResponse {
	t.Helper()
	schemaResp := resource.SchemaResponse{}
	target.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("could not build target schema: %v", schemaResp.Diagnostics)
	}
	return &resource.MoveStateResponse{
		TargetState: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
}

// TestMoveStateAcceptsReleasedState is the test that matters for the rename: a
// `moved` block from each former type name must carry real, already-released
// state onto the current schema.
func TestMoveStateAcceptsReleasedState(t *testing.T) {
	ctx := context.Background()

	for _, test := range moveTests {
		t.Run(test.name, func(t *testing.T) {
			target := test.newTarget()
			mover, ok := target.(resource.ResourceWithMoveState)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithMoveState", target)
			}

			movers := mover.MoveState(ctx)
			if len(movers) != 1 {
				t.Fatalf("got %d movers, want 1", len(movers))
			}

			resp := moveResponse(ctx, t, target)
			movers[0].StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      test.sourceType,
				SourceSchemaVersion: 0,
				SourceRawState:      &tfprotov6.RawState{JSON: []byte(test.priorState)},
			}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("moving from %s failed: %v", test.sourceType, resp.Diagnostics)
			}
			// A null target state is how the framework detects a mover that
			// declined the request, so it would mean the move silently did not
			// happen.
			if resp.TargetState.Raw.IsNull() {
				t.Fatalf("mover declined to move state from %s", test.sourceType)
			}

			var moved struct {
				Id string `tfsdk:"id"`
			}
			if diags := resp.TargetState.GetAttribute(ctx, path.Root("id"), &moved.Id); diags.HasError() {
				t.Fatalf("moved state has no readable id: %v", diags)
			}
			if moved.Id == "" {
				t.Error("moved state lost its id")
			}
		})
	}
}

// TestMoveStateDeclinesOtherResources guards the type-name literal each resource
// passes: a mover that accepted anything would silently move unrelated state,
// and one with a typo would silently accept nothing.
func TestMoveStateDeclinesOtherResources(t *testing.T) {
	ctx := context.Background()

	for _, test := range moveTests {
		t.Run(test.name, func(t *testing.T) {
			target := test.newTarget()
			mover, ok := target.(resource.ResourceWithMoveState)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithMoveState", target)
			}
			movers := mover.MoveState(ctx)

			resp := moveResponse(ctx, t, target)
			movers[0].StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      "p0_some_other_resource",
				SourceSchemaVersion: 0,
				SourceRawState:      &tfprotov6.RawState{JSON: []byte(test.priorState)},
			}, resp)

			if resp.Diagnostics.HasError() {
				t.Errorf("declining a move must not error: %v", resp.Diagnostics)
			}
			if !resp.TargetState.Raw.IsNull() {
				t.Error("mover accepted state from an unrelated resource type")
			}
		})
	}
}
