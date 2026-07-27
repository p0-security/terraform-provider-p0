// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package settings

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// Ensure roleBinding satisfies the framework interfaces.
var _ resource.Resource = &roleBinding{}
var _ resource.ResourceWithImportState = &roleBinding{}
var _ resource.ResourceWithConfigure = &roleBinding{}

// roleBinding is a generic Terraform resource that binds a single principal (a
// user or a directory group) to a single P0 role. Each exported constructor in
// roles.go fixes the role/kind/attribute so that every P0 role surfaces as its
// own strongly-typed resource.
//
// The P0 settings API exposes no read endpoint for role bindings (the app reads
// them via Firestore subscriptions), so Read is a passthrough of prior state:
// out-of-band changes are not detected.
type roleBinding struct {
	data *internal.P0ProviderData

	// typeName is the resource type suffix, e.g. "owner_user".
	typeName string
	// role is the P0 backend role slug, e.g. "owner", "manager", "iamOwner".
	role string
	// kind is the binding collection: "users" or "groups".
	kind string
	// attr is the single schema attribute name: "email" or "group".
	attr string
	// attrDescription documents the single attribute.
	attrDescription string
	// markdownDescription documents the resource.
	markdownDescription string
}

func (r *roleBinding) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}

func (r *roleBinding) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.markdownDescription,
		Attributes: map[string]schema.Attribute{
			r.attr: schema.StringAttribute{
				MarkdownDescription: r.attrDescription,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *roleBinding) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	if data != nil {
		r.data = data
	}
}

// bindingPath builds the settings API path for the given principal.
func (r *roleBinding) bindingPath(value string) string {
	return fmt.Sprintf("settings/roles/%s/bindings/%s/%s", r.role, r.kind, url.PathEscape(value))
}

// normalize lower-cases user emails to match P0's server-side normalization,
// which avoids a permanent diff between config and stored state.
func (r *roleBinding) normalize(value string) string {
	if r.kind == "users" {
		return strings.ToLower(value)
	}
	return value
}

func (r *roleBinding) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	diag := &resp.Diagnostics

	var value types.String
	diag.Append(req.Plan.GetAttribute(ctx, path.Root(r.attr), &value)...)
	if diag.HasError() {
		return
	}

	normalized := r.normalize(value.ValueString())

	// Role-binding writes take no request body and return an empty 201/204.
	_, err := r.data.Put(r.bindingPath(normalized), nil, nil)
	if err != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to add %s to role %q:\n%s", normalized, r.role, err))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Added %s binding %q to role %q", r.kind, normalized, r.role))

	// Persist the configured value verbatim: normalization is only for the API
	// path, and (since there is no read endpoint) state must equal config to
	// avoid a "provider produced inconsistent result after apply" error.
	diag.Append(resp.State.SetAttribute(ctx, path.Root(r.attr), value)...)
}

func (r *roleBinding) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// The P0 settings API has no read endpoint for role bindings; preserve prior
	// state as-is.
	var value types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(r.attr), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.attr), value)...)
}

func (r *roleBinding) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// The single attribute forces replacement, so Update only runs when nothing
	// changed. Carry the planned value through to state.
	var value types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(r.attr), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.attr), value)...)
}

func (r *roleBinding) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	diag := &resp.Diagnostics

	var value types.String
	diag.Append(req.State.GetAttribute(ctx, path.Root(r.attr), &value)...)
	if diag.HasError() {
		return
	}

	normalized := r.normalize(value.ValueString())

	_, err := r.data.Delete(r.bindingPath(normalized))
	if err != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to remove %s from role %q:\n%s", normalized, r.role, err))
	}
}

func (r *roleBinding) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root(r.attr), req, resp)
}
