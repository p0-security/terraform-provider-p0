package internal

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// IsMoveFrom reports whether a MoveResourceState request comes from the given
// resource type at the given schema version. A StateMover should leave its
// response untouched when this is false, so that an unrecognized source is
// reported as "implementation not found" rather than moving unrelated state.
func IsMoveFrom(req resource.MoveStateRequest, sourceTypeName string, sourceSchemaVersion int64) bool {
	// The provider address is deliberately unchecked: it varies across mirrors
	// and forks, while type name plus schema version already pin the state shape.
	return req.SourceTypeName == sourceTypeName && req.SourceSchemaVersion == sourceSchemaVersion
}

// RenamedFrom builds the StateMover list for a resource type that was renamed,
// to be returned from the renamed resource's MoveState method. Pass the former
// type name and the schema version it was last published at, e.g.
// ("p0_agentic_gateway", 0). It copies every attribute the target schema still
// declares and drops the rest, so it suits a resource whose model can decode
// that result; one whose model cannot, or whose data moved to a new attribute,
// needs its own mover.
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
			if !IsMoveFrom(req, sourceTypeName, sourceSchemaVersion) || req.SourceRawState == nil {
				return
			}

			// IgnoreUndefinedAttributes matches how the framework itself reads
			// prior state, so a rename never rejects state that a plain refresh
			// would accept.
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
