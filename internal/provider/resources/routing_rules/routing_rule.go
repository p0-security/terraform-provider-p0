// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package routingrules

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &RoutingRule{}
var _ resource.ResourceWithImportState = &RoutingRule{}

type RoutingRule struct {
	data *internal.P0ProviderData
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type RoutingRuleJson struct {
	Name      *string         `json:"name" tfsdk:"name"`
	Requestor RequestorModel  `json:"requestor" tfsdk:"requestor"`
	Resource  ResourceModel   `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModel `json:"approval" tfsdk:"approval"`
}

type UpdateRoutingRule struct {
	Rule RoutingRuleModel `json:"rule"`
}

func NewRoutingRule() resource.Resource {
	return &RoutingRule{}
}

func getPath(name string) string {
	encodedName := url.PathEscape(name)
	return fmt.Sprintf("routing/name/%s", encodedName)
}

func toJson(model RoutingRuleModel) RoutingRuleJson {
	return RoutingRuleJson{
		Name:      model.Name,
		Requestor: *model.Requestor,
		Resource:  *model.Resource,
		Approval:  model.Approval}
}

func toModel(json RoutingRuleJson) RoutingRuleModel {
	return RoutingRuleModel{
		Name:      json.Name,
		Requestor: &json.Requestor,
		Resource:  &json.Resource,
		Approval:  json.Approval,
	}
}

func (rule *RoutingRule) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rule"
}

func (rule *RoutingRule) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: `A routing rule that controls who can request access to what, and access requirements.
See [the P0 request-routing docs](https://docs.p0.dev/just-in-time-access/request-routing).`,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the rule",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"requestor": requestorAttribute,
			"resource":  resourceAttribute,
			"approval":  approvalAttribute,
		},
	}
}

func (rule *RoutingRule) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		rule.data = data
	}
}

func (rule *RoutingRule) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var diag = &resp.Diagnostics

	// Load the plan into the model
	var model RoutingRuleModel
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rule to create: %+v", model))

	// Create the routing rule
	var updated RoutingRuleModel
	_, postErr := rule.data.Post(getPath(*model.Name), &model, &updated)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to create routing rule:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest routing rule: %+v", updated))

	// Update the Terraform state to reflect the newly created routing rule
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), updated.Name)...)
	diag.Append(resp.State.Set(ctx, updated)...)
}

func (rule *RoutingRule) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var diag = &resp.Diagnostics

	// Load the state into the model
	var model RoutingRuleModel
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	// Read the routing rule
	var json RoutingRuleJson
	httpResponse, httpErr := rule.data.Get(getPath(*model.Name), &json)
	if httpErr != nil {
		// Check if the error indicates that the resource was not found (404)
		if httpResponse.StatusCode == 404 {
			tflog.Debug(ctx, "Routing rule not found (404), removing from state")
			// Remove the resource from state by calling RemoveResource.
			resp.State.RemoveResource(ctx)
			return
		}

		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to read routing rule:\n%s", httpErr))
		return
	}

	model = toModel(json)

	// Update the Terraform state to match the routing rule returned by the API
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), model.Name)...)
	diag.Append(resp.State.Set(ctx, model)...)

	tflog.Debug(ctx, fmt.Sprintf("Reading routing rule: %+v", model))
}

func (rule *RoutingRule) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var diag = &resp.Diagnostics

	// Load the plan into the model
	var model RoutingRuleModel
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rule to update: %+v", model))

	// Read the current routing rule from the Terraform state
	var currentModel RoutingRuleModel
	diag.Append(req.State.Get(ctx, &currentModel)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Current routing rule state: %+v", currentModel))

	json := toJson(model)

	// Update the routing rule
	var updatedJson RoutingRuleJson
	_, postErr := rule.data.Put(getPath(*model.Name), &json, &updatedJson)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update routing rule:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated routing rule: %+v", updatedJson))

	updatedModel := toModel(updatedJson)

	// Update the Terraform state to reflect the updated routing rule
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), updatedModel.Name)...)
	diag.Append(resp.State.Set(ctx, updatedModel)...)
}

func (rule *RoutingRule) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var diag = &resp.Diagnostics

	// Load the state into the model
	var model RoutingRuleModel
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	// Delete the routing rule
	_, postErr := rule.data.Delete(getPath(*model.Name))
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to delete routing rule:\n%s", postErr))
	}
}

func (rule *RoutingRule) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
