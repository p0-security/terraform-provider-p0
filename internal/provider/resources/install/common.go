package installresources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	Config                      = "configure"
	Verify                      = "verify"
	AwsLabelMarkdownDescription = "The AWS account's alias (if available)"
	StateMarkdownDescription    = `This account's install progress in the P0 application:
		- 'stage': The account has been staged for installation
		- 'configure': The account is available to be added to P0, and may be configured
		- 'installed': The account is fully installed`
	IamWrite  = "iam-write"
	Inventory = "inventory"
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
	// The provider internal data object
	ProviderData *internal.P0ProviderData
	// Get the path for the item in the API
	GetItemPath func(id string) string
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

// Advances the item to "installed" state.
//
// To use, the item's TFSDK model must be passed. For example:
//
//	var data ItemConfigurationModel
//	var json ConfigurationApiResponseJson
//	install.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
func (i *Install) Upsert(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, json any, data any) {
	diags.Append(plan.Get(ctx, data)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(data)
	if id == nil {
		i.reportConversionError("Missing ID", "Could not extract ID from", data, diags)
		return
	}

	inputJson := i.ToJson(data)
	if inputJson == nil {
		i.reportConversionError("Bad Terraform state", "Could not represent as JSON", data, diags)
		return
	}

	for _, step := range InstallSteps {
		// in-place evolves data object
		path := fmt.Sprintf("%s/%s", i.GetItemPath(*id), step)
		err := i.ProviderData.Post(path, inputJson, json)
		if err != nil {
			diags.AddError("Error communicating with P0", fmt.Sprintf("Failed %s step, got error:\n%s", step, err))
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
func (i *Install) Read(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, json any, data any) {
	diags.Append(state.Get(ctx, data)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(data)
	if id == nil {
		i.reportConversionError("Missing ID", "Could not extract ID from", data, diags)
		return
	}

	httpErr := i.ProviderData.Get(i.GetItemPath(*id), json)
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

// "Delete" does not delete the item from P0; rather, it returns it to the "stage" state.
//
// This prevents double-delete issues when the stage resource is also deleted.
func (i *Install) Delete(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, data any) {
	diags.Append(state.Get(ctx, data)...)
	if diags.HasError() {
		return
	}

	id := i.GetId(data)
	if id == nil {
		i.reportConversionError("Missing ID", "Could not extract ID from", data, diags)
		return
	}

	json := i.ToJson(data)
	if json == nil {
		i.reportConversionError("Bad Terraform state", "Could not create an API request from", json, diags)
		return
	}

	var discardedResponse = struct{}{}
	httpErr := i.ProviderData.Put(i.GetItemPath(*id), json, &discardedResponse)
	if httpErr != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not delete, got error:\n%s", httpErr))
		return
	}
}
