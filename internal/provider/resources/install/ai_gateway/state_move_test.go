package installaigateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// moveTestProvider serves only this package's resources, so the tests below go
// through the framework's real MoveResourceState handling rather than an
// imitation of it.
type moveTestProvider struct{}

func (p *moveTestProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "p0"
}

func (p *moveTestProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
}

func (p *moveTestProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
}

func (p *moveTestProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewGateway, NewGatewayStaged, NewServer}
}

func (p *moveTestProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

// The prior states below are what v0.53.0 wrote under the former type names.
// They deliberately differ from the current schemas: v0.53.0's top-level `url`
// and `oauth_endpoint` were since replaced by `domain_hosting`.
var moveTests = []struct {
	name       string
	newTarget  func() resource.Resource
	newModel   func() any
	sourceType string
	targetType string
	priorState string
}{
	{
		name:       "gateway",
		newTarget:  NewGateway,
		newModel:   func() any { return &gatewayModel{} },
		sourceType: "p0_agentic_gateway",
		targetType: "p0_ai_gateway",
		priorState: `{"id":"primary","url":"https://gateway.example.com",` +
			`"oauth_endpoint":"https://oauth.gateway.example.com",` +
			`"service_account_email":"p0@example.iam.gserviceaccount.com"}`,
	},
	{
		name:       "staged gateway",
		newTarget:  NewGatewayStaged,
		newModel:   func() any { return &gatewayStagedModel{} },
		sourceType: "p0_agentic_gateway_staged",
		targetType: "p0_ai_gateway_staged",
		priorState: `{"id":"primary","url":"https://gateway.example.com",` +
			`"service_account_email":"p0@example.iam.gserviceaccount.com"}`,
	},
	{
		name:       "server",
		newTarget:  NewServer,
		newModel:   func() any { return &serverModel{} },
		sourceType: "p0_agentic_server",
		targetType: "p0_ai_gateway_server",
		priorState: `{"id":"aws-tools","gateway":"primary"}`,
	},
}

// moveState sends a MoveResourceState request through the framework, as
// Terraform does for a `moved` block that changes the resource type.
func moveState(ctx context.Context, t *testing.T, sourceType, targetType, priorState string) *tfprotov6.MoveResourceStateResponse {
	t.Helper()
	server := providerserver.NewProtocol6(&moveTestProvider{})()
	resp, err := server.MoveResourceState(ctx, &tfprotov6.MoveResourceStateRequest{
		SourceProviderAddress: "registry.terraform.io/p0-security/p0",
		SourceTypeName:        sourceType,
		SourceSchemaVersion:   0,
		SourceState:           &tfprotov6.RawState{JSON: []byte(priorState)},
		TargetTypeName:        targetType,
	})
	if err != nil {
		t.Fatalf("MoveResourceState: %v", err)
	}
	return resp
}

// movedState decodes a successful move's target state under the target's schema.
func movedState(ctx context.Context, t *testing.T, target resource.Resource, resp *tfprotov6.MoveResourceStateResponse) tfsdk.State {
	t.Helper()
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("move failed: %s: %s", d.Summary, d.Detail)
		}
	}
	if resp.TargetState == nil {
		t.Fatal("move returned no target state")
	}

	schemaResp := resource.SchemaResponse{}
	target.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	raw, err := resp.TargetState.Unmarshal(schemaResp.Schema.Type().TerraformType(ctx))
	if err != nil {
		t.Fatalf("target state does not match the target schema: %v", err)
	}
	return tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
}

// TestMoveStateAcceptsReleasedState is the test that matters for the rename: a
// `moved` block from each former type name must carry released state onto the
// current schema, in a form the resource can go on to refresh.
func TestMoveStateAcceptsReleasedState(t *testing.T) {
	ctx := context.Background()

	for _, test := range moveTests {
		t.Run(test.name, func(t *testing.T) {
			resp := moveState(ctx, t, test.sourceType, test.targetType, test.priorState)
			state := movedState(ctx, t, test.newTarget(), resp)

			// Read begins by decoding prior state into the resource's model, so
			// moved state it cannot decode fails every refresh after the move.
			if diags := state.Get(ctx, test.newModel()); diags.HasError() {
				t.Fatalf("Read cannot decode the moved state: %v", diags)
			}
		})
	}
}

// TestMoveStateDeclinesOtherResources guards the type-name literal each resource
// matches on: a mover that accepted anything would move unrelated state, and
// one with a typo would accept nothing.
func TestMoveStateDeclinesOtherResources(t *testing.T) {
	ctx := context.Background()

	for _, test := range moveTests {
		t.Run(test.name, func(t *testing.T) {
			resp := moveState(ctx, t, "p0_some_other_resource", test.targetType, test.priorState)

			if resp.TargetState != nil {
				t.Error("mover accepted state from an unrelated resource type")
			}
			if len(resp.Diagnostics) == 0 {
				t.Error("an unmatched move should be reported, not silently ignored")
			}
		})
	}
}

// v0.53.0 kept the staged gateway's url at the top level; it now lives under
// domain_hosting, and a move that dropped it would leave the gateway urlless.
func TestMoveStateCarriesStagedUrlIntoDomainHosting(t *testing.T) {
	ctx := context.Background()
	resp := moveState(ctx, t, "p0_agentic_gateway_staged", "p0_ai_gateway_staged",
		`{"id":"primary","url":"https://gateway.example.com"}`)
	state := movedState(ctx, t, NewGatewayStaged(), resp)

	var moved gatewayStagedModel
	if diags := state.Get(ctx, &moved); diags.HasError() {
		t.Fatalf("moved state does not decode: %v", diags)
	}
	if moved.Id != "primary" {
		t.Errorf("id: got %q, want primary", moved.Id)
	}
	if moved.DomainHosting == nil {
		t.Fatal("moved state has no domain_hosting")
	}
	if moved.DomainHosting.Url != "https://gateway.example.com" {
		t.Errorf("domain_hosting.url: got %q, want the released url", moved.DomainHosting.Url)
	}
	// selfHosted is the attribute's default and only supported value, so
	// anything else would register as a change to domain_hosting.
	if moved.DomainHosting.Type.ValueString() != "selfHosted" {
		t.Errorf("domain_hosting.type: got %q, want selfHosted", moved.DomainHosting.Type.ValueString())
	}
}
