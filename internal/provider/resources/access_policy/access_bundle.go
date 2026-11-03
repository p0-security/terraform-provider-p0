// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

var _ resource.Resource = &AccessBundle{}
var _ resource.ResourceWithImportState = &AccessBundle{}
var _ resource.ResourceWithModifyPlan = &AccessBundle{}

// The item limits of the bundles API's validateBundle.
const (
	minBundleItems = 2
	maxBundleItems = 15
)

// P0 trims the label, so a label with surrounding whitespace would never match
// what P0 stores.
var trimmedLabelRegex = regexp.MustCompile(`^\S(?:.*\S)?$`)

type AccessBundle struct {
	data *internal.P0ProviderData
}

type AccessBundleItemModel struct {
	Type   string               `tfsdk:"type"`
	Access string               `tfsdk:"access"`
	Args   jsontypes.Normalized `tfsdk:"args"`
}

type AccessBundleModel struct {
	Id          types.String            `tfsdk:"id"`
	Label       *string                 `tfsdk:"label"`
	Description *string                 `tfsdk:"description"`
	Disabled    *bool                   `tfsdk:"disabled"`
	Requestor   *RequestorModelV5       `tfsdk:"requestor"`
	Approval    []ApprovalModelV3       `tfsdk:"approval"`
	Items       []AccessBundleItemModel `tfsdk:"items"`
	PolicyName  types.String            `tfsdk:"policy_name"`
	CreatedBy   types.String            `tfsdk:"created_by"`
	UpdatedAt   types.Int64             `tfsdk:"updated_at"`
}

type AccessBundleItemJson struct {
	Type   string          `json:"type"`
	Access string          `json:"access"`
	Args   json.RawMessage `json:"args"`
}

// AccessBundleJson is a bundle as the bundles API sends and receives it. The
// API keeps an empty approval list as given, so Approval is a pointer: nil
// leaves it out, while an empty slice sends [].
type AccessBundleJson struct {
	Id          string                 `json:"id,omitempty"`
	Label       string                 `json:"label"`
	Description *string                `json:"description,omitempty"`
	Disabled    *bool                  `json:"disabled,omitempty"`
	Requestor   *RequestorJson         `json:"requestor,omitempty"`
	Approval    *[]ApprovalModelV3     `json:"approval,omitempty"`
	Items       []AccessBundleItemJson `json:"items"`
	CreatedBy   string                 `json:"createdBy,omitempty"`
	UpdatedAt   int64                  `json:"updatedAt,omitempty"`
}

type accessBundleResponse struct {
	Bundle AccessBundleJson `json:"bundle"`
}

func NewAccessBundle() resource.Resource {
	return &AccessBundle{}
}

func bundlePolicyName(id string) string {
	return bundlePolicyPrefix + id
}

func bundlePath(id string) string {
	return fmt.Sprintf("bundles/%s", url.PathEscape(id))
}

func bundleToJson(model AccessBundleModel) AccessBundleJson {
	bundle := AccessBundleJson{
		Label:       *model.Label,
		Description: model.Description,
		Disabled:    model.Disabled,
		Items:       make([]AccessBundleItemJson, len(model.Items)),
	}
	if model.Requestor != nil {
		requestor := requestorToJson(model.Requestor)
		bundle.Requestor = &requestor
	}
	if model.Approval != nil {
		bundle.Approval = &model.Approval
	}
	for i, item := range model.Items {
		bundle.Items[i] = AccessBundleItemJson{
			Type:   item.Type,
			Access: item.Access,
			Args:   json.RawMessage(item.Args.ValueString()),
		}
	}
	return bundle
}

func bundleToModel(bundle AccessBundleJson) AccessBundleModel {
	model := AccessBundleModel{
		Id:         types.StringValue(bundle.Id),
		Label:      &bundle.Label,
		Disabled:   bundle.Disabled,
		Items:      make([]AccessBundleItemModel, len(bundle.Items)),
		PolicyName: types.StringValue(bundlePolicyName(bundle.Id)),
		CreatedBy:  types.StringValue(bundle.CreatedBy),
		UpdatedAt:  types.Int64Value(bundle.UpdatedAt),
	}
	// The editor saves an empty description when none is given; the schema
	// doesn't allow one, so read it as unset.
	if bundle.Description != nil && *bundle.Description != "" {
		model.Description = bundle.Description
	}
	if bundle.Requestor != nil {
		model.Requestor = requestorFromJson(*bundle.Requestor)
	}
	if bundle.Approval != nil {
		model.Approval = *bundle.Approval
	}
	for i, item := range bundle.Items {
		args := string(item.Args)
		if len(item.Args) == 0 {
			args = "{}"
		}
		model.Items[i] = AccessBundleItemModel{
			Type:   item.Type,
			Access: item.Access,
			Args:   jsontypes.NewNormalizedValue(args),
		}
	}
	return model
}

func (bundle *AccessBundle) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_bundle"
}

func (bundle *AccessBundle) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	requestor := requestorAttribute(currentSchemaVersion)
	requestor.Required = false
	requestor.Optional = true
	requestor.MarkdownDescription = `Who may request the bundle. If unset, anyone may request it. See [the Requestor docs](https://docs.p0.dev/just-in-time-access/request-routing#requestor).`

	approval := approvalAttribute(currentSchemaVersion)
	approval.Required = false
	approval.Optional = true
	approval.MarkdownDescription = `How requests for the bundle are approved. If unset or empty, a P0 security reviewer must approve them. See [the Approval docs](https://docs.p0.dev/just-in-time-access/request-routing#approval).`

	resp.Schema = schema.Schema{
		MarkdownDescription: `An access bundle: a set of access items that users request together, in one request.

Each bundle owns a policy rule named ` + "`bundle:<id>`" + `, which P0 writes from the bundle's ` + "`requestor`" + `, ` + "`approval`" + ` and ` + "`disabled`" + ` settings.

This resource is in preview: set ` + "`enable_preview_resources = true`" + ` in the provider block to use it.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The bundle's ID, assigned by P0.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "The bundle's name, as requestors see it.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(trimmedLabelRegex, "must not be empty or start or end with whitespace"),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A description of the bundle, as requestors see it.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the bundle can't be requested. Defaults to false.",
				Optional:            true,
			},
			"requestor": requestor,
			"approval":  approval,
			"items": schema.ListNestedAttribute{
				MarkdownDescription: fmt.Sprintf("The access the bundle grants, from %d to %d items. P0 keeps them in the order given.", minBundleItems, maxBundleItems),
				Required:            true,
				Validators: []validator.List{
					listvalidator.SizeBetween(minBundleItems, maxBundleItems),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							MarkdownDescription: "The integration that grants this item, for example `gcloud` or `aws`.",
							Required:            true,
						},
						"access": schema.StringAttribute{
							MarkdownDescription: "The kind of access within the integration, for example `role` or `permission-set`.",
							Required:            true,
						},
						"args": schema.StringAttribute{
							MarkdownDescription: "The item's arguments, as a JSON object. Use `jsonencode`. Key order and whitespace don't matter.",
							CustomType:          jsontypes.NormalizedType{},
							Required:            true,
						},
					},
				},
			},
			"policy_name": schema.StringAttribute{
				MarkdownDescription: "The name of the policy rule P0 keeps for this bundle, `bundle:<id>`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_by": schema.StringAttribute{
				MarkdownDescription: "Who created the bundle: a user's email address, or an API key's name.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.Int64Attribute{
				MarkdownDescription: "When the bundle last changed, in milliseconds since the Unix epoch.",
				Computed:            true,
			},
		},
	}
}

func (bundle *AccessBundle) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		bundle.data = data
	}
}

// ModifyPlan refuses to plan a bundle unless preview resources are enabled.
// Destroying one is still allowed, so turning the flag off doesn't strand it.
func (bundle *AccessBundle) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	internal.RequirePreviewResources(bundle.data, "p0_access_bundle", &resp.Diagnostics)
}

func (bundle *AccessBundle) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var diag = &resp.Diagnostics

	var model AccessBundleModel
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Access bundle to create: %+v", model))

	bundle.data.PolicyWriteLock.Lock()
	defer bundle.data.PolicyWriteLock.Unlock()
	var created accessBundleResponse
	_, postErr := bundle.data.Post("bundles", bundleToJson(model), &created)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to create access bundle:\n%s", postErr))
		return
	}

	diag.Append(resp.State.Set(ctx, bundleToModel(created.Bundle))...)
}

func (bundle *AccessBundle) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var diag = &resp.Diagnostics

	var model AccessBundleModel
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	var read accessBundleResponse
	httpResponse, httpErr := bundle.data.Get(bundlePath(model.Id.ValueString()), &read)
	if httpErr != nil {
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			tflog.Debug(ctx, "Access bundle not found (404), removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to read access bundle:\n%s", httpErr))
		return
	}

	diag.Append(resp.State.Set(ctx, bundleToModel(read.Bundle))...)
}

func (bundle *AccessBundle) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var diag = &resp.Diagnostics

	var model AccessBundleModel
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Access bundle to update: %+v", model))

	bundle.data.PolicyWriteLock.Lock()
	defer bundle.data.PolicyWriteLock.Unlock()
	var updated accessBundleResponse
	_, putErr := bundle.data.Put(bundlePath(model.Id.ValueString()), bundleToJson(model), &updated)
	if putErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update access bundle:\n%s", putErr))
		return
	}

	diag.Append(resp.State.Set(ctx, bundleToModel(updated.Bundle))...)
}

func (bundle *AccessBundle) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var diag = &resp.Diagnostics

	var model AccessBundleModel
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	bundle.data.PolicyWriteLock.Lock()
	defer bundle.data.PolicyWriteLock.Unlock()
	_, deleteErr := bundle.data.Delete(bundlePath(model.Id.ValueString()))
	if deleteErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to delete access bundle:\n%s", deleteErr))
	}
}

func (bundle *AccessBundle) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	internal.RequirePreviewResources(bundle.data, "p0_access_bundle", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
