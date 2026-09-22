package internal

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// RenamedFrom builds the StateMover list for a resource type that was renamed,
// to be returned from the renamed resource's MoveState method. Pass the former
// type name and the schema version it was last published at, e.g.
// ("p0_agentic_gateway", 0). It moves state only between types that share a
// schema, so it suits a pure rename; a rename that also changes the schema needs
// its own mover that transforms the prior model.
//
// Given this, a user migrates by editing the type name and adding:
//
//	moved {
//	  from = p0_agentic_gateway.example
//	  to   = p0_ai_gateway.example
//	}
//
// Terraform 1.8 is the minimum version that issues this request.
func RenamedFrom(sourceTypeName string, sourceSchemaVersion int64) []resource.StateMover {
	return []resource.StateMover{{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			// Leaving the response untouched skips this mover, so an
			// unrecognized source is reported as "implementation not found"
			// rather than moving unrelated state. The provider address is
			// deliberately unchecked: it varies across mirrors and forks, while
			// type name plus schema version already pin the state shape.
			if req.SourceTypeName != sourceTypeName ||
				req.SourceSchemaVersion != sourceSchemaVersion ||
				req.SourceRawState == nil {
				return
			}

			// The framework pre-populates TargetState with the target's schema,
			// and the rename left that schema alone, so the source state decodes
			// as-is and needs no transformation.
			//
			// IgnoreUndefinedAttributes matches what the framework itself does
			// when it reads prior state, so a rename never rejects state that a
			// plain refresh would have accepted: a resource whose schema drifted
			// under a stable version still moves, and is then reconciled by the
			// same plan that would have reconciled it without the rename.
			rawState, err := req.SourceRawState.UnmarshalWithOpts(
				resp.TargetState.Schema.Type().TerraformType(ctx),
				tfprotov6.UnmarshalOpts{
					ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
				},
			)
			if err != nil {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					"Could not read prior state of "+sourceTypeName+": "+err.Error()+
						". Please report this issue to support@p0.dev.",
				)
				return
			}

			resp.TargetState.Raw = rawState
			resp.TargetPrivate = req.SourcePrivate
		},
	}}
}
