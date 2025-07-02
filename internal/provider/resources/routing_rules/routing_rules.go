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

type RoutingRulesModelV0 struct {
	Rule    []RoutingRuleModelV0 `tfsdk:"rule"`
	Version types.String         `tfsdk:"version"`
}

type RoutingRulesModelV1 struct {
	Rule    []RoutingRuleModelV1 `tfsdk:"rule"`
	Version types.String         `tfsdk:"version"`
}

type RoutingRulesModelV2 struct {
	Rule    []RoutingRuleModelV2 `tfsdk:"rule"`
	Version types.String         `tfsdk:"version"`
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type LatestRoutingRules struct {
	Rule    []RoutingRuleModelV2 `json:"rules"`
	Version *string              `json:"version"`
}

type WorkflowLatestApi struct {
	Workflow LatestRoutingRules `json:"workflow"`
}

type UpdateRoutingRules struct {
	Rule []RoutingRuleModelV2 `json:"rules"`
}

type WorkflowUpdateApi struct {
	Workflow       UpdateRoutingRules `json:"workflow"`
	CurrentVersion *string            `json:"currentVersion"`
}

var defaultRoutingRules = LatestRoutingRules{
	Rule: []RoutingRuleModelV2{{
		Requestor: &RequestorModelV2{Type: "any"},
		Resource:  &ResourceModel{Type: "any"},
		Approval: []ApprovalModelV2{{
			Type:    "p0",
			Options: &ApprovalOptionsModel{AllowOneParty: &False, RequireReason: &False, BreakGlassApprover: &False}}},
	}},
}

func newMultiRuleSchema(version int64) schema.Schema {
	return schema.Schema{
		Version: currentSchemaVersion,
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: `The rules that control who can request access to what, and access requirements.
See [the P0 request-routing docs](https://docs.p0.dev/just-in-time-access/request-routing).`,
		Blocks: map[string]schema.Block{
			"rule": schema.SetNestedBlock{
				MarkdownDescription: "All access rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the rule",
							Required:            true,
						},
						"requestor": requestorAttribute(version),
						"resource":  resourceAttribute,
						"approval":  approvalAttribute(version),
					},
				},
			},
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

func (rules *RoutingRules) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rules"
}

func (rules *RoutingRules) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = newMultiRuleSchema(currentSchemaVersion)
}

func (rules *RoutingRules) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		rules.data = data
	}
}

// Updates TF state based on current state and P0 routing-rules API response.
func (rules *RoutingRules) updateState(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, data RoutingRulesModelV2, latest WorkflowLatestApi) {
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
func (rules *RoutingRules) postVersion(ctx context.Context, model RoutingRulesModelV2, diag *diag.Diagnostics, state *tfsdk.State) {
	tflog.Debug(ctx, fmt.Sprintf("Routing rules to update: %+v", model))

	// Read the current routing rules from the Terraform state
	var currentModel RoutingRulesModelV2
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
	_, postErr := rules.data.Post("routing", &toUpdate, &updated)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update routing rules, got error:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest routing rules: %+v", updated))

	// Update the Terraform state to reflect the updated routing rules
	rules.updateState(ctx, diag, state, model, updated)
}

func (rules *RoutingRules) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var diag = &resp.Diagnostics

	var model RoutingRulesModelV2

	// Load the data from the plan into the model
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rules to create: %+v", model))

	// Even if we are replacing the rules, it is technically an update, so retrieve the current routing rules
	var current WorkflowLatestApi
	_, httpErr := rules.data.Get("routing/latest", &current)
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
	_, postErr := rules.data.Post("routing", &toUpdate, &updated)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update routing rules, got error:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest routing rules: %+v", updated))

	// Update the Terraform state to reflect the newly created routing rules
	rules.updateState(ctx, diag, &resp.State, model, updated)
}

func (rules *RoutingRules) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var diag = &resp.Diagnostics

	var data RoutingRulesModelV2
	diag.Append(req.State.Get(ctx, &data)...)
	if diag.HasError() {
		return
	}

	var latest WorkflowLatestApi
	_, httpErr := rules.data.Get("routing/latest", &latest)
	if httpErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to read routing rules, got error:\n%s", httpErr))
		return
	}

	rules.updateState(ctx, diag, &resp.State, data, latest)

	tflog.Debug(ctx, fmt.Sprintf("Reading latest workflow: %+v", latest))
}

func (rules *RoutingRules) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RoutingRulesModelV2
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules.postVersion(ctx, data, &resp.Diagnostics, &resp.State)
}

func (rules *RoutingRules) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var diag = &resp.Diagnostics

	var data RoutingRulesModelV2
	diag.Append(req.State.Get(ctx, &data)...)
	if diag.HasError() {
		return
	}

	diag.AddWarning(
		"Routing rules are not deleted",
		`Routing rules can not be deleted. Deleting the routing_rules resource instead restores rules to the P0 default rules.
These rules allow all principals to request access to all resources, with manual approval by P0 approvers.`,
	)

	// Set workflow to default rules
	data.Rule = defaultRoutingRules.Rule

	rules.postVersion(ctx, data, diag, &resp.State)
}

func (rules *RoutingRules) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("version"), req, resp)
}

func (rule *RoutingRules) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	var schemaV0 = newMultiRuleSchema(0)
	var schemaV1 = newMultiRuleSchema(1)
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schemaV0,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior RoutingRulesModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				upgradedRules := make([]RoutingRuleModelV1, len(prior.Rule))

				for i, rule := range prior.Rule {
					requestor := upgradeRequestorV0(rule.Requestor)
					upgradedRules[i] = RoutingRuleModelV1{
						Name:      rule.Name,
						Requestor: &requestor,
						Resource:  rule.Resource,
						Approval:  upgradeApprovalV0(rule.Approval),
					}
				}

				upgraded := RoutingRulesModelV1{
					Rule:    upgradedRules,
					Version: prior.Version,
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, upgraded)...)
			},
		},
		1: {
			PriorSchema: &schemaV1,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior RoutingRulesModelV1
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgradedRules := make([]RoutingRuleModelV2, len(prior.Rule))
				for i, rule := range prior.Rule {
					requestor := upgradeRequestorV1(rule.Requestor)
					upgradedRules[i] = RoutingRuleModelV2{
						Name:      rule.Name,
						Requestor: &requestor,
						Resource:  rule.Resource,
						Approval:  upgradeApprovalV1(rule.Approval),
					}
				}

				upgraded := RoutingRulesModelV2{
					Rule:    upgradedRules,
					Version: prior.Version,
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, upgraded)...)
			},
		},
	}
}
