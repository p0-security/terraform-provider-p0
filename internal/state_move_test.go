package internal

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const renameSourceType = "p0_agentic_gateway"

var renameTestSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id":  schema.StringAttribute{Required: true},
		"url": schema.StringAttribute{Required: true},
	},
}

// newRenameResponse mirrors how the framework pre-populates the response: the
// target's schema, and a null value of that schema's type.
func newRenameResponse(ctx context.Context) *resource.MoveStateResponse {
	targetType := renameTestSchema.Type().TerraformType(ctx)
	return &resource.MoveStateResponse{
		TargetState: tfsdk.State{
			Schema: renameTestSchema,
			Raw:    tftypes.NewValue(targetType, nil),
		},
	}
}

func callRenameMover(ctx context.Context, req resource.MoveStateRequest) *resource.MoveStateResponse {
	movers := RenamedFrom(renameSourceType, 0)
	resp := newRenameResponse(ctx)
	movers[0].StateMover(ctx, req, resp)
	return resp
}

func TestRenamedFromCopiesStateVerbatim(t *testing.T) {
	ctx := context.Background()
	resp := callRenameMover(ctx, resource.MoveStateRequest{
		SourceTypeName:      renameSourceType,
		SourceSchemaVersion: 0,
		SourceRawState: &tfprotov6.RawState{
			JSON: []byte(`{"id":"primary","url":"https://gateway.example.com"}`),
		},
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics, got %v", resp.Diagnostics)
	}

	var moved struct {
		Id  string `tfsdk:"id"`
		Url string `tfsdk:"url"`
	}
	if diags := resp.TargetState.Get(ctx, &moved); diags.HasError() {
		t.Fatalf("moved state did not decode under the target schema: %v", diags)
	}
	if moved.Id != "primary" {
		t.Errorf("id: got %q, want %q", moved.Id, "primary")
	}
	if moved.Url != "https://gateway.example.com" {
		t.Errorf("url: got %q, want %q", moved.Url, "https://gateway.example.com")
	}
}

// A skipped mover leaves the response untouched; that is what tells the
// framework to try the next mover, and ultimately to report that no
// implementation matched rather than move unrelated state.
func TestRenamedFromSkipsUnmatchedSource(t *testing.T) {
	ctx := context.Background()
	rawState := &tfprotov6.RawState{
		JSON: []byte(`{"id":"primary","url":"https://gateway.example.com"}`),
	}

	for _, test := range []struct {
		name string
		req  resource.MoveStateRequest
	}{
		{
			name: "another resource type",
			req: resource.MoveStateRequest{
				SourceTypeName:      "p0_agentic_server",
				SourceSchemaVersion: 0,
				SourceRawState:      rawState,
			},
		},
		{
			name: "same type at a different schema version",
			req: resource.MoveStateRequest{
				SourceTypeName:      renameSourceType,
				SourceSchemaVersion: 1,
				SourceRawState:      rawState,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := callRenameMover(ctx, test.req)

			if resp.Diagnostics.HasError() {
				t.Errorf("a skipped mover must not report errors, got %v", resp.Diagnostics)
			}
			if !resp.TargetState.Raw.IsNull() {
				t.Errorf("a skipped mover must leave target state null, got %v", resp.TargetState.Raw)
			}
		})
	}
}

func TestRenamedFromReportsUnreadablePriorState(t *testing.T) {
	ctx := context.Background()
	resp := callRenameMover(ctx, resource.MoveStateRequest{
		SourceTypeName:      renameSourceType,
		SourceSchemaVersion: 0,
		SourceRawState:      &tfprotov6.RawState{JSON: []byte(`{"id":`)},
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for unreadable prior state")
	}
}

func TestRenamedFromSkipsNilPriorState(t *testing.T) {
	ctx := context.Background()
	resp := callRenameMover(ctx, resource.MoveStateRequest{
		SourceTypeName:      renameSourceType,
		SourceSchemaVersion: 0,
		SourceRawState:      nil,
	})

	if resp.Diagnostics.HasError() {
		t.Errorf("a skipped mover must not report errors, got %v", resp.Diagnostics)
	}
	if !resp.TargetState.Raw.IsNull() {
		t.Error("a skipped mover must leave target state null")
	}
}

// TestRenamedFromDropsUndeclaredAttributes covers state written before the
// target schema dropped an attribute: the move must still succeed, as a plain
// refresh of that state would.
func TestRenamedFromDropsUndeclaredAttributes(t *testing.T) {
	ctx := context.Background()
	resp := callRenameMover(ctx, resource.MoveStateRequest{
		SourceTypeName:      renameSourceType,
		SourceSchemaVersion: 0,
		SourceRawState: &tfprotov6.RawState{
			JSON: []byte(`{"id":"primary","url":"https://gateway.example.com","oauth_endpoint":"https://oauth.example.com"}`),
		},
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected undeclared attributes to be dropped, got %v", resp.Diagnostics)
	}
	if resp.TargetState.Raw.IsNull() {
		t.Fatal("mover declined state that carried an undeclared attribute")
	}
}
