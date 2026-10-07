package common

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	Config                   = "configure"
	Verify                   = "verify"
	SingletonKey             = "_"
	StateMarkdownDescription = `This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed`
)

// An item's install states in P0, as its 'state' attribute holds them.
const (
	StateStage     = "stage"
	StateConfigure = "configure"
	StateInstalled = "installed"
)

var StateAttribute = schema.StringAttribute{
	Computed:            true,
	MarkdownDescription: StateMarkdownDescription,
}

// A ModifyPlan for a resource that installs its item. It plans an update for an item that
// P0 hasn't installed, even when the configuration hasn't changed, so that the next apply
// finishes the install: the resource's Update picks up from the step P0 has the item at.
// That covers an item imported before P0 finished installing it, and one that a failed
// apply left unfinished. An installed item keeps the framework's plan, which shows no
// difference while the configuration matches it.
func PlanFinishingInstall(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == StateInstalled {
		return
	}

	// As in any update the framework plans, the state, which only P0 sets, is unknown
	// until apply. Update takes it from P0's response, which has the item's new state.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
}

// Order matters here; components installed in this order.
var InstallSteps = []string{Verify, Config}

type AwsPartition struct {
	Type *string `json:"type"`
}

type Install struct {
	// This Integration's key
	Integration string
	// This Component's key
	Component string
	// The provider internal data object
	ProviderData *internal.P0ProviderData
	// Extract the item id from the TF state model, or nil if it can not be extracted
	GetId func(data any) *string
	// Convert the API response to the single item's JSON (should just equate to returning &data.Item)
	GetItemJson func(readJson any) any
	// Convert a pointer to the item's JSON model to a pointer to the TF state model.
	// Returns nil if the JSON can not be converted.
	FromJson func(ctx context.Context, diags *diag.Diagnostics, id string, json any) any
	// Convert a pointer to the TF state model to a pointer to an item's JSON model
	ToJson func(data any) any
	// Optional. Describes an install check that failed on the verify or configure step,
	// in place of the generic "Error communicating with P0" diagnostic. P0 runs some
	// integrations' checks through a connector the customer deploys, so a failure
	// usually points at the customer's own setup, which P0's message, carried in err,
	// describes.
	DescribeCheckError func(id string, err error) (summary string, detail string)
}

func (i *Install) itemPath(id string) string {
	return fmt.Sprintf("%s/%s/%s", i.itemBasePath(), i.Component, id)
}

func (i *Install) itemBasePath() string {
	return fmt.Sprintf("integrations/%s/config", i.Integration)
}

// Ensures that the item's configuration has been created in P0. If the item's configuration already exists we'll ignore the error.
func (i *Install) EnsureConfig(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, model any) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	throwaway_response := struct{}{}
	_, err := i.ProviderData.Post(i.itemBasePath(), struct{}{}, &throwaway_response)
	if err != nil {
		// we can safely ignore 409 Conflict errors, because they indicate the item is already installed
		if !strings.Contains(err.Error(), "409 Conflict") {
			diags.AddError("Error communicating with P0", fmt.Sprintf("Failed to install integration %s, got error %s", i.Integration, err))
			return
		}
	}
}

// Places the item in the "stage" state in P0.
// To use, the item's TFSDK model must be passed. For example:
//
//	var data ItemConfigurationModel
//	var json ConfigurationApiResponseJson
func (i *Install) Stage(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any, inputJson any) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	_, err := i.ProviderData.Put(i.itemPath(*id), inputJson, json)
	if err != nil {
		diags.AddError(fmt.Sprintf("Could not stage %s component", i.Component), fmt.Sprintf("Error: %s", err))
		return
	}

	itemJson := i.GetItemJson(json)
	if itemJson == nil {
		reportConversionError("Bad API response", "Could not read 'item' from", json, diags)
		return
	}

	created := i.FromJson(ctx, diags, *id, itemJson)
	if created == nil {
		// FromJson may have already reported a specific diagnostic
		if !diags.HasError() {
			reportConversionError("Bad API response", "Could not read resource data from", itemJson, diags)
		}
		return
	}

	diags.Append(state.Set(ctx, created)...)
}

// Advances the item to "installed" state.
//
// To use, the item's TFSDK model must be passed. For example:
//
//	var data ItemConfigurationModel
//	var json ConfigurationApiResponseJson
//	install.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
func (i *Install) UpsertFromStage(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any) {
	i.upsert(ctx, diags, plan, state, json, model, InstallSteps)
}

// Advances an item that P0 has already verified, one at "configure" or "installed", to
// "installed" with the configure step alone, as UpsertFromStage does from "stage".
//
// P0 saves an item only once the step's installer has accepted the planned values,
// merged over the stored item. So a failed step leaves both the item and, in an Update,
// the resource's state as they were. An integration that checks on configure all that
// it checks on verify can use this in Update to make the update all-or-nothing.
func (i *Install) UpsertFromConfigure(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any) {
	i.upsert(ctx, diags, plan, state, json, model, []string{Config})
}

// Posts the planned item to each of steps in turn, and sets state to the item that the
// last step returns.
func (i *Install) upsert(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any, steps []string) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	inputJson := i.ToJson(model)
	if inputJson == nil {
		reportConversionError("Bad Terraform state", "Could not represent as JSON", model, diags)
		return
	}

	for _, step := range steps {
		// in-place evolves data object
		path := fmt.Sprintf("%s/%s", i.itemPath(*id), step)
		resp, err := i.ProviderData.Post(path, inputJson, json)
		if err != nil {
			// A failed step leaves the state as it was, a 404 included: removing the
			// resource here would have the framework report a provider bug in place of
			// this error. The next apply replaces the resource, or recreates it once a
			// refresh finds it gone.
			diags.AddError(i.stepError(*id, step, resp, err))
			return
		}
	}

	itemJson := i.GetItemJson(json)
	if itemJson == nil {
		reportConversionError("Bad API response", "Could not read 'item' from", json, diags)
		return
	}

	updated := i.FromJson(ctx, diags, *id, itemJson)
	if updated == nil {
		// FromJson may have already reported a specific diagnostic
		if !diags.HasError() {
			reportConversionError("Bad API response", "Could not read resource data from", itemJson, diags)
		}
		return
	}

	diags.Append(state.Set(ctx, updated)...)
}

// The diagnostic for an install step that failed.
//
// P0 answers a step with a 404 only when the item doesn't exist, so it was removed
// while this apply ran. A failed install check, on either step, is a 400 or a 422, or
// a 502 when something the check calls, such as a connector, couldn't be reached.
// Those go to DescribeCheckError, if set. Anything else, such as an authorization
// error, a P0 server error or a request that never reached P0, gets the generic
// diagnostic.
func (i *Install) stepError(id string, step string, resp *http.Response, err error) (string, string) {
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}

	switch {
	case status == http.StatusNotFound:
		return fmt.Sprintf("Could not %s %s component", step, i.Component),
			fmt.Sprintf("P0 has no %s item %q. It may have been removed while Terraform was applying this change. Apply again to recreate it.", i.Component, id)
	case i.DescribeCheckError != nil && (status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || status == http.StatusBadGateway):
		return i.DescribeCheckError(id, err)
	default:
		return "Error communicating with P0", fmt.Sprintf("Could not %s %s component, got error:\n%s", step, i.Component, err)
	}
}

// Reads current item value.
//
// 'json' must be a pointer to a struct of form:
//
//	struct{
//	  Item *ItemConfigurationJson `json:"item"`
//	}
func (i *Install) Read(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, json any, model any) {
	diags.Append(state.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	resp, httpErr := i.ProviderData.Get(i.itemPath(*id), json)
	if resp != nil && resp.StatusCode == 404 {
		state.RemoveResource(ctx)
		return
	}
	if httpErr != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to read configuration, got error:\n%s", httpErr))
		return
	}

	itemJson := i.GetItemJson(json)
	if itemJson == nil {
		reportConversionError("Bad API response", "Could not read 'item' from", json, diags)
		return
	}

	updated := i.FromJson(ctx, diags, *id, itemJson)
	if updated == nil {
		// FromJson may have already reported a specific diagnostic
		if !diags.HasError() {
			reportConversionError("Bad API response", "Could not read resource data from", itemJson, diags)
		}
		return
	}

	diags.Append(state.Set(ctx, updated)...)
}

// "Rollback" does not delete the item from P0; rather, it returns it to the "stage" state.
//
// This prevents double-delete issues when the stage resource is also deleted.
func (i *Install) Rollback(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, model any) {
	diags.Append(state.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	json := i.ToJson(model)
	if json == nil {
		reportConversionError("Bad Terraform state", "Could not create an API request from", model, diags)
		return
	}

	var discardedResponse = struct{}{}
	_, httpErr := i.ProviderData.Put(i.itemPath(*id), json, &discardedResponse)
	if httpErr != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not rollback, got error:\n%s", httpErr))
		return
	}
}

// Deletes the item from P0.
func (i *Install) Delete(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, model any) {
	diags.Append(state.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	resp, err := i.ProviderData.Delete(i.itemPath(*id))
	if resp != nil && resp.StatusCode == 404 {
		// Item was already removed.
		return
	}
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not delete, got error: %s", err))
		return
	}
}
