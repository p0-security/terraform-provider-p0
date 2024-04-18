package installresources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	Config                   = "configure"
	Verify                   = "verify"
	IamWrite                 = "iam-write"
	Inventory                = "inventory"
	StateMarkdownDescription = `This account's install progress in the P0 application:
	- 'stage': The account has been staged for installation
	- 'configure': The account is available to be added to P0, and may be configured
	- 'installed': The account is fully installed`
)

// Order matters here; components installed in this order.
var InstallSteps = []string{Verify, Config}

func OperationPath(base_path string, step string) string {
	return fmt.Sprintf("%s/%s", base_path, step)
}

type ReadResponse struct {
	Item *any
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
	// Convert a pointer to the item's JSON model to a pointer to the TF state model
	FromJson func(id string, json any) any
	// Convert a pointer to the TF state model to a pointer to an item's JSON model
	ToJson func(data any) any
}

func (i *Install) reportConversionError(header string, subheader string, value any, diags *diag.Diagnostics) {
	marshalled, marshallErr := json.MarshalIndent(value, "", "  ")
	if marshallErr != nil {
		marshalled = []byte("<An unparseable entity>")
	}
	diags.AddError(header, fmt.Sprintf("%s:\n%s", subheader, marshalled))
}

func (i *Install) itemPath(id string) string {
	return fmt.Sprintf("%s/%s/%s", i.itemBasePath(), i.Component, id)
}

func (i *Install) itemBasePath() string {
	return fmt.Sprintf("integrations/%s/config", i.Integration)
}

// Places the item in the "stage" state in P0.
// To use, the item's TFSDK model must be passed. For example:
//
//	var data ItemConfigurationModel
//	var json ConfigurationApiResponseJson
func (i *Install) Upsert(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	throwaway_response := struct{}{}
	err := i.ProviderData.Post(i.itemBasePath(), struct{}{}, &throwaway_response)
	if err != nil {
		// we can safely 409 Conflict errors, because they indicate the item is already installed
		if !strings.Contains(err.Error(), "409 Conflict") {
			diags.AddError("Error communicating with P0", fmt.Sprintf("Failed to install integration %s, got error %s", i.Integration, err))
			return
		}
	}
}

// Advances the item to "installed" state.
//
// To use, the item's TFSDK model must be passed. For example:
//
//	var data ItemConfigurationModel
//	var json ConfigurationApiResponseJson
//	install.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
func (i *Install) UpsertFromStage(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, model any) {
	diags.Append(plan.Get(ctx, model)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(model)
	if id == nil {
		i.reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	inputJson := i.ToJson(model)
	if inputJson == nil {
		i.reportConversionError("Bad Terraform state", "Could not represent as JSON", model, diags)
		return
	}

	for _, step := range InstallSteps {
		// in-place evolves data object
		path := fmt.Sprintf("%s/%s", i.itemPath(*id), step)
		err := i.ProviderData.Post(path, inputJson, json)
		if err != nil {
			diags.AddError("Error communicating with P0", fmt.Sprintf("Could not %s %s component, got error:\n%s", step, i.Component, err))
			return
		}
	}

	itemJson := i.GetItemJson(json)
	if itemJson == nil {
		i.reportConversionError("Bad API response", "Could not read 'item' from", json, diags)
		return
	}

	updated := i.FromJson(*id, itemJson)
	if updated == nil {
		i.reportConversionError("Bad API response", "Could not read resource data from", itemJson, diags)
		return
	}

	diags.Append(state.Set(ctx, updated)...)
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
		i.reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	httpErr := i.ProviderData.Get(i.itemPath(*id), json)
	if httpErr != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to read configuration, got error:\n%s", httpErr))
		return
	}

	itemJson := i.GetItemJson(json)
	if itemJson == nil {
		i.reportConversionError("Bad API response", "Could not read 'item' from", json, diags)
		return
	}

	updated := i.FromJson(*id, itemJson)
	if updated == nil {
		i.reportConversionError("Bad API response", "Could not read resource data from", itemJson, diags)
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
		i.reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	json := i.ToJson(model)
	if json == nil {
		i.reportConversionError("Bad Terraform state", "Could not create an API request from", json, diags)
		return
	}

	var discardedResponse = struct{}{}
	httpErr := i.ProviderData.Put(i.itemPath(*id), json, &discardedResponse)
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
		i.reportConversionError("Missing ID", "Could not extract ID from", model, diags)
		return
	}

	// delete the staged component.
	err := i.ProviderData.Delete(i.itemPath(*id))
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not delete, got error: %s", err))
		return
	}
}
