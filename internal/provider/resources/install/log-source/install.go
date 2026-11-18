// Package installlogsource holds what the Access Logs log-source resources
// share: p0_aws_cloudwatch_logs and the Google Cloud Logging resources. Each
// log-source item deploys a connector of its own, which P0 checks on the verify
// step, and P0 generates the names the connector's Terraform needs when the
// item is staged.
//
// The plan and update logic is a copy of the Redshift (AI) install resources'
// (see redshift-ai/install.go).
package installlogsource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ends the description of each log-source resource.
const BetaNote = `**Note:** Access Logs is in beta. P0 offers this resource only to organizations that have Access Logs turned on.`

// The install API's response for one item.
type ItemApi[T any] struct {
	Item *T `json:"item"`
}

// GetItemJson for every log-source component.
func GetItemJson[T any](json any) any {
	api, ok := json.(*ItemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

// A required string that P0 fixes once the item exists: the item's key, or a
// field marked `step: "new"` in the app's install schema, which P0 refuses to
// change after staging. RequiresReplace plans the destroy-and-create that works,
// in place of an update that P0 would refuse.
func FixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// A string that P0 generates, and that stays as it is until the item is
// replaced.
func GeneratedAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// Creates the integration in P0 if it doesn't exist yet, then stages the item.
func EnsureConfigAndStage(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	installer.EnsureConfig(ctx, diags, plan, state, model)
	if diags.HasError() {
		return
	}
	installer.Stage(ctx, diags, plan, state, api, model, installer.ToJson(model))
}

// Stages the item, then verifies and configures it.
func StageAndInstall(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	EnsureConfigAndStage(ctx, installer, diags, plan, state, api, model)
	if diags.HasError() {
		return
	}
	installer.UpsertFromStage(ctx, diags, plan, state, api, model)
}

// A ModifyPlan for a resource that installs its item. It plans an update for an
// item that P0 hasn't installed, even when the configuration hasn't changed, so
// that the next apply finishes the install. That covers an item imported before
// P0 finished installing it, and one that a failed apply left unfinished. An
// installed item keeps the framework's plan.
//
// generated names the resource's GeneratedAttribute fields that P0 sets only
// when it installs the item. One that the state has no value for is unknown in
// the plan, so that the install's value can fill it.
func PlanFinishingInstall(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, generated ...path.Path) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)

	for _, attribute := range generated {
		var value types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, attribute, &value)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if value.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, attribute, types.StringUnknown())...)
		}
	}
}

// Update for a resource that installs its item, whose every configurable
// attribute requires replacement. So an update only finishes an install that
// PlanFinishingInstall planned, from the step that P0 has the item at: an item
// that P0 has verified gets the configure step alone, and any other item is
// verified first, as Create does.
func UpdateFromInstallState(ctx context.Context, installer *common.Install, req resource.UpdateRequest, resp *resource.UpdateResponse, api any, model any) {
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

// Imports an item into attribute, once the import ID passes the check that the
// configuration's value gets. The read that follows an import puts the key in a
// URL path, where the provider's client crashes on a path that doesn't parse.
func ImportById(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, attribute path.Path, validators ...validator.String) {
	for _, v := range validators {
		result := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        attribute,
			ConfigValue: types.StringValue(req.ID),
		}, result)
		resp.Diagnostics.Append(result.Diagnostics...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resource.ImportStatePassthroughID(ctx, attribute, req, resp)
}

// P0 runs each log source's install check through the customer's connector, so
// a failed check usually means a problem in their cloud setup, which P0's
// message describes.
func DescribeCheckError(label string, component string) common.DescribeError {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("%s install check failed", label),
			fmt.Sprintf("P0 rejected the install check for the %s %s %q:\n\n%s", label, component, id, err)
	}
}

// P0 answers a write with a 404 while it doesn't offer Access Logs to the
// organization, since the log-source components are behind its feature flag.
func DescribeWriteNotFound(label string, component string) common.DescribeError {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("P0 doesn't offer the %s %s", label, component),
			fmt.Sprintf("P0 couldn't find the %s %s %q. P0 offers log sources only to organizations that have Access Logs turned on; contact support@p0.dev to turn it on. If Access Logs is on, the item may have been removed while Terraform was applying this change: apply again to recreate it.\n\n%s", label, component, id, err)
	}
}
