package installredshiftai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A ModifyPlan for a Redshift (AI) resource that installs its item, as
// p0_github_repositories plans. It plans an update for an item that P0 hasn't
// installed, even when the configuration hasn't changed, so that the next apply
// finishes the install: the resource's Update runs the steps P0 hasn't. That
// covers an item imported before P0 finished installing it, and one that a
// failed apply left unfinished. An installed item keeps the framework's plan,
// which shows no difference while the configuration matches it.
func planFinishingInstall(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	// As in any update the framework plans, the state, which only P0 sets, is
	// unknown until apply. Update takes it from P0's response, which has the item's
	// new state.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
}

// Update for a Redshift (AI) resource that installs its item. It applies the
// plan from the step that P0 has the item at, so it also finishes an install
// that planFinishingInstall plans an update for.
//
// An item that P0 has verified, at configure or installed, gets the configure
// step alone (see common.Install.UpsertFromConfigure). P0 saves nothing when the
// step fails, so a failed update leaves the item, and the resource's state, as
// they were. That suits these resources, whose configure step checks every
// attribute that changes in place: any other attribute requires replacement, so
// a verified item passed verify with the values it still has. Any other item,
// such as a staged one, is verified first, as Create does.
func updateFromInstallState(ctx context.Context, installer *common.Install, req resource.UpdateRequest, resp *resource.UpdateResponse, api any, model any) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, api, model)
	default:
		installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, api, model)
	}
}

// Imports an item by its key, once the key passes the check that the
// configuration's id gets. The read that follows an import puts the key in a URL
// path, where `12#34` reads the item `12`, and the provider's client crashes on
// a path that doesn't parse, such as one with `12%zz`.
func importById(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, id validator.String) {
	result := &validator.StringResponse{}
	id.ValidateString(ctx, validator.StringRequest{
		Path:        path.Root("id"),
		ConfigValue: types.StringValue(req.ID),
	}, result)
	resp.Diagnostics.Append(result.Diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
