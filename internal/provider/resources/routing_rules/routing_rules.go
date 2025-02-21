// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package routingrules

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &RoutingRules{}
var _ resource.ResourceWithImportState = &RoutingRules{}

func NewRoutingRules() resource.Resource {
	return &RoutingRules{}
}

type RoutingRules struct {
	data *internal.P0ProviderData
}

type RoutingRulesModel struct {
	Rule    []RoutingRuleModel `tfsdk:"rule"`
	Version types.String       `tfsdk:"version"`
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type LatestRoutingRules struct {
	Rule    []RoutingRuleModel `json:"rules"`
	Version *string            `json:"version"`
}

type WorkflowLatestApi struct {
	Workflow LatestRoutingRules `json:"workflow"`
}

type UpdateRoutingRules struct {
	Rule []RoutingRuleModel `json:"rules"`
}

type WorkflowUpdateApi struct {
	Workflow       UpdateRoutingRules `json:"workflow"`
	CurrentVersion *string            `json:"currentVersion"`
}

var defaultRoutingRules = LatestRoutingRules{
	Rule: []RoutingRuleModel{{
		Requestor: RequestorModel{Type: "any"},
		Resource:  ResourceModel{Type: "any"},
		Approval: []ApprovalModel{{
			Type:    "p0",
			Options: &ApprovalOptionsModel{AllowOneParty: &False, RequireReason: &False}}},
	}},
}

func (r *RoutingRules) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rules"
}

func (r *RoutingRules) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: `The rules that control who can request access to what, and access requirements.
See [the P0 request-routing docs](https://docs.p0.dev/just-in-time-access/request-routing).`,
		Blocks: map[string]schema.Block{
			"rule": ruleNestedBlock,
		},
		Attributes: map[string]schema.Attribute{
			"version": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Rules-document static version",
			},
		},
	}
}

func (r *RoutingRules) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		r.data = data
	}
}

// Updates TF state based on current state and P0 routing-rules API response.
func (r *RoutingRules) updateState(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, data RoutingRulesModel, latest WorkflowLatestApi) {
	if latest.Workflow.Version == nil {
		diags.AddError("Missing routing rules version", "P0 did not return a version for routing rules; please report this to support@p0.dev.")
		return
	}

	data.Rule = latest.Workflow.Rule
	data.Version = types.StringValue(*latest.Workflow.Version)

	tflog.Debug(ctx, fmt.Sprintf("Updating state to: %+v", data))

	// Save updated data into Terraform state
	diags.Append(state.Set(ctx, data)...)
}

// Posts a new routing-rules version to P0. This is used for update and delete.
// Note that delete does not delete, but rather posts a default routing-rules set.
func (r *RoutingRules) postVersion(ctx context.Context, model RoutingRulesModel, diag *diag.Diagnostics, state *tfsdk.State) {
	tflog.Debug(ctx, fmt.Sprintf("Routing rules to update: %+v", model))

	// Read the current routing rules from the Terraform state
	var currentModel RoutingRulesModel
	diag.Append(state.Get(ctx, &currentModel)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Current routing rules state: %+v", currentModel))

	// Grab the current version number from the model
	var currentVersionPtr *string
	if !currentModel.Version.IsUnknown() && !currentModel.Version.IsNull() {
		currentVersion := currentModel.Version.ValueString()
		tflog.Debug(ctx, fmt.Sprintf("Current routing rules version: %s", currentVersion))
		currentVersionPtr = &currentVersion
	}

	// Convert the model to the API format
	toUpdate := WorkflowUpdateApi{Workflow: UpdateRoutingRules{Rule: model.Rule}, CurrentVersion: currentVersionPtr}

	tflog.Debug(ctx, fmt.Sprintf("Updated routing rules: %+v", toUpdate))

	// Update the routing rules
	var updated WorkflowLatestApi
	_, postErr := r.data.Post("routing", &toUpdate, &updated)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update routing rules, got error:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest routing rules: %+v", updated))

	// Update the Terraform state to reflect the updated routing rules
	r.updateState(ctx, diag, state, model, updated)
}

func (r *RoutingRules) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var diag = &resp.Diagnostics

	var model RoutingRulesModel

	// Load the data from the plan into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rules to create: %+v", model))

	// Even if we are replacing the rules, it is technically an update, so retrieve the current routing rules
	var current WorkflowLatestApi
	_, httpErr := r.data.Get("routing/latest", &current)
	if httpErr != nil {
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Unable to read routing rules, got error:\n%s", httpErr))
		return
	}

	// ... and grab the current version
	var currentVersionPtr = current.Workflow.Version

	tflog.Debug(ctx, fmt.Sprintf("Current routing rules version: %s", *currentVersionPtr))

	// Convert the model to the API format
	toUpdate := WorkflowUpdateApi{Workflow: UpdateRoutingRules{Rule: model.Rule}, CurrentVersion: currentVersionPtr}

	tflog.Debug(ctx, fmt.Sprintf("Updated routing rules: %+v", toUpdate))

	// Update the routing rules
	var updated WorkflowLatestApi
	_, postErr := r.data.Post("routing", &toUpdate, &updated)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update routing rules, got error:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest routing rules: %+v", updated))

	// Update the Terraform state to reflect the newly created routing rules
	r.updateState(ctx, diag, &resp.State, model, updated)
}

func (r *RoutingRules) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoutingRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var latest WorkflowLatestApi
	_, httpErr := r.data.Get("routing/latest", &latest)
	if httpErr != nil {
		resp.Diagnostics.AddError("Error communicating with P0", fmt.Sprintf("Unable to read routing rules, got error:\n%s", httpErr))
		return
	}

	r.updateState(ctx, &resp.Diagnostics, &resp.State, data, latest)

	tflog.Debug(ctx, fmt.Sprintf("Reading latest workflow: %+v", latest))
}

func (r *RoutingRules) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RoutingRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.postVersion(ctx, data, &resp.Diagnostics, &resp.State)
}

func (r *RoutingRules) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RoutingRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddWarning(
		"Routing rules are not deleted",
		`Routing rules can not be deleted. Deleting the routing_rules resource instead restores rules to the P0 default rules.
These rules allow all principals to request access to all resources, with manual approval by P0 approvers.`,
	)

	// Set workflow to default rules
	data.Rule = defaultRoutingRules.Rule

	r.postVersion(ctx, data, &resp.Diagnostics, &resp.State)
}

func (r *RoutingRules) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("version"), req, resp)
}
