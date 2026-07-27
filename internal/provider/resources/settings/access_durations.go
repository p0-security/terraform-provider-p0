// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package settings

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

var _ resource.Resource = &AccessDurations{}
var _ resource.ResourceWithConfigure = &AccessDurations{}

// AccessDurations manages an organization's access-duration policy. It is a
// singleton: declare it at most once per P0 organization.
//
// The three durations live in a single P0 configuration document but are set
// via three separate endpoints. The P0 API has no read endpoint (the app reads
// this config via Firestore subscriptions), so Read is a passthrough of prior
// state and `terraform destroy` leaves the last-applied values in place.
type AccessDurations struct {
	data *internal.P0ProviderData
}

func NewAccessDurations() resource.Resource {
	return &AccessDurations{}
}

type accessDurationsModel struct {
	Approvable     *durationOption `tfsdk:"approvable"`
	MaxAccess      *durationOption `tfsdk:"max_access"`
	StandingAccess *durationOption `tfsdk:"standing_access"`
}

// durationEndpoints maps each model duration to its settings API endpoint.
func (m *accessDurationsModel) durationEndpoints() []struct {
	endpoint string
	option   *durationOption
} {
	return []struct {
		endpoint string
		option   *durationOption
	}{
		{"settings/approvable-duration", m.Approvable},
		{"settings/max-access-duration", m.MaxAccess},
		{"settings/standing-access-duration", m.StandingAccess},
	}
}

func (r *AccessDurations) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_durations"
}

func (r *AccessDurations) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An organization's access-duration policy. This is a singleton resource; declare it at most once.

The P0 API does not expose a read endpoint for these settings, so Terraform cannot detect changes made outside of Terraform (drift), and ` + "`terraform destroy`" + ` leaves the last-applied values in place.`,
		Attributes: map[string]schema.Attribute{
			"approvable":      durationAttribute("The maximum amount of time between when a request is made and when it can be approved."),
			"max_access":      durationAttribute("The maximum duration for which access may be granted."),
			"standing_access": durationAttribute("The maximum duration of standing (persistent) access before it must be re-approved."),
		},
	}
}

func (r *AccessDurations) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	if data != nil {
		r.data = data
	}
}

// apply writes all three durations to P0.
func (r *AccessDurations) apply(ctx context.Context, diags *diag.Diagnostics, model *accessDurationsModel) {
	for _, d := range model.durationEndpoints() {
		var response map[string]any
		_, err := r.data.Put(d.endpoint, d.option, &response)
		if err != nil {
			diags.AddError("Error communicating with P0", fmt.Sprintf("Unable to set %s:\n%s", d.endpoint, err))
			return
		}
		tflog.Debug(ctx, fmt.Sprintf("Set %s to %+v", d.endpoint, d.option))
	}
}

func (r *AccessDurations) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model accessDurationsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &resp.Diagnostics, &model)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *AccessDurations) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// No read endpoint; preserve prior state as-is.
	var model accessDurationsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *AccessDurations) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model accessDurationsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &resp.Diagnostics, &model)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *AccessDurations) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// The P0 API has no endpoint to unset these durations; removing the resource
	// only drops it from Terraform state and leaves the last-applied values.
	tflog.Debug(ctx, "Deleting p0_access_durations from state; P0-side values are left unchanged")
}
