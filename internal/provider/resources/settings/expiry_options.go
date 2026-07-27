// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package settings

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

var _ resource.Resource = &ExpiryOptions{}
var _ resource.ResourceWithConfigure = &ExpiryOptions{}

// ExpiryOptions manages the selectable request-duration presets ("expiry
// options") an organization offers. It is a singleton: declare it at most once.
//
// P0 identifies each option by a derived label (see computeValue) and de-dupes
// options by (time, unit). The API has no read endpoint, so Read is a
// passthrough of prior state.
type ExpiryOptions struct {
	data *internal.P0ProviderData
}

func NewExpiryOptions() resource.Resource {
	return &ExpiryOptions{}
}

type expiryOptionsModel struct {
	Options []durationOption `tfsdk:"options"`
}

func (r *ExpiryOptions) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_expiry_options"
}

func (r *ExpiryOptions) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The selectable request-duration presets ("expiry options") offered to requestors. This is a singleton resource; declare it at most once.

Options are de-duplicated by their ` + "`time`" + ` and ` + "`unit`" + `. The P0 API does not expose a read endpoint for these settings, so Terraform cannot detect changes made outside of Terraform (drift).`,
		Attributes: map[string]schema.Attribute{
			"options": schema.ListNestedAttribute{
				MarkdownDescription: "The list of selectable request durations.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: durationAttributes(),
				},
			},
		},
	}
}

func (r *ExpiryOptions) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	if data != nil {
		r.data = data
	}
}

// optionKey uniquely identifies an option by its (time, unit), matching P0's
// de-duplication.
func optionKey(o durationOption) string {
	return fmt.Sprintf("%d/%s", o.Time, o.Unit)
}

func (r *ExpiryOptions) add(ctx context.Context, diags *diag.Diagnostics, o durationOption) {
	var response map[string]any
	_, err := r.data.Post("settings/expiry-options", &o, &response)
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to add expiry option %+v:\n%s", o, err))
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("Added expiry option %+v", o))
}

func (r *ExpiryOptions) remove(ctx context.Context, diags *diag.Diagnostics, o durationOption) {
	key := url.PathEscape(computeValue(o.Time, o.Unit))
	_, err := r.data.Delete("settings/expiry-options/" + key)
	if err != nil {
		diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to remove expiry option %+v:\n%s", o, err))
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("Removed expiry option %+v", o))
}

func (r *ExpiryOptions) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model expiryOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, o := range model.Options {
		r.add(ctx, &resp.Diagnostics, o)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *ExpiryOptions) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// No read endpoint; preserve prior state as-is.
	var model expiryOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *ExpiryOptions) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan expiryOptionsModel
	var state expiryOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	priorKeys := map[string]bool{}
	for _, o := range state.Options {
		priorKeys[optionKey(o)] = true
	}
	planKeys := map[string]bool{}
	for _, o := range plan.Options {
		planKeys[optionKey(o)] = true
	}

	// Add options present in the plan but not in prior state.
	for _, o := range plan.Options {
		if !priorKeys[optionKey(o)] {
			r.add(ctx, &resp.Diagnostics, o)
			if resp.Diagnostics.HasError() {
				return
			}
		}
	}

	// Remove options present in prior state but not in the plan.
	for _, o := range state.Options {
		if !planKeys[optionKey(o)] {
			r.remove(ctx, &resp.Diagnostics, o)
			if resp.Diagnostics.HasError() {
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ExpiryOptions) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model expiryOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, o := range model.Options {
		r.remove(ctx, &resp.Diagnostics, o)
	}
}
