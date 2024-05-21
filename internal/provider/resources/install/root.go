package installresources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/p0-security/terraform-provider-p0/internal"
)

type RootInstall struct {
	// This Integration's key
	Integration string
	// The provider internal data object
	ProviderData *internal.P0ProviderData
	// Convert a pointer to the item's JSON model to a pointer to the TF state model
	FromJson func(json any) any
	// Convert a pointer to the TF state model to a pointer to an item's JSON model
	ToJson func(data any) any
}

func (i *RootInstall) configPath() string {
	return fmt.Sprintf("integrations/%s/config", i.Integration)
}

// Creates the integration + root item in P0.
func (i *RootInstall) Create(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	inputJson := i.ToJson(model)

	err := i.ProviderData.Post(i.configPath(), &inputJson, &json)
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Failed to install integration %s, got error %s", i.Integration, err))
		return
	}

	item := i.FromJson(json)
	if item == nil {
		reportConversionError("Bad API response", "Could not read resource data from", json, diags)
		return
	}

	diags.Append(state.Set(ctx, item)...)
}

// Reads the integration from P0.
func (i *RootInstall) Read(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, json any, model any) {
	diags.Append(state.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	err := i.ProviderData.Get(i.configPath(), &json)
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Failed to install integration %s, got error %s", i.Integration, err))
		return
	}

	item := i.FromJson(json)
	if item == nil {
		reportConversionError("Bad API response", "Could not read resource data from", json, diags)
		return
	}

	diags.Append(state.Set(ctx, item)...)
}

// Deletes the integration from P0.
func (i *RootInstall) Delete(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, model any) {
	diags.Append(state.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	// delete the staged component.
	err := i.ProviderData.Delete(i.configPath())
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not delete, got error: %s", err))
		return
	}
}
