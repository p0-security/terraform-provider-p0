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
var _ resource.ResourceWithUpgradeState = &RoutingRule{}

type RoutingRule struct {
	data *internal.P0ProviderData
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type RoutingRuleJson struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Requestor RequestorModelV2  `json:"requestor" tfsdk:"requestor"`
	Resource  ResourceModel     `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV2 `json:"approval" tfsdk:"approval"`
}

type UpdateRoutingRule struct {
	Rule RoutingRuleModelV2 `json:"rule"`
}

func NewRoutingRule() resource.Resource {
	return &RoutingRule{}
}

func getPath(name string) string {
	encodedName := url.PathEscape(name)
	return fmt.Sprintf("routing/name/%s", encodedName)
}

func toJson(model RoutingRuleModelV2) RoutingRuleJson {
	return RoutingRuleJson{
		Name:      model.Name,
		Requestor: *model.Requestor,
		Resource:  *model.Resource,
		Approval:  model.Approval}
}

func toModel(json RoutingRuleJson) RoutingRuleModelV2 {
	return RoutingRuleModelV2{
		Name:      json.Name,
		Requestor: &json.Requestor,
		Resource:  &json.Resource,
		Approval:  json.Approval,
	}
}

func newSingleRuleSchema(version int64) schema.Schema {
	return schema.Schema{
		Version: version,
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
			"requestor": requestorAttribute(version),
			"resource":  resourceAttribute,
			"approval":  approvalAttribute(version),
		},
	}
}

func (rule *RoutingRule) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rule"
}

func (rule *RoutingRule) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = newSingleRuleSchema(currentSchemaVersion)
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
	var model RoutingRuleModelV2
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rule to create: %+v", model))

	// Create the routing rule
	var updated RoutingRuleModelV2
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
	var model RoutingRuleModelV2
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
	var model RoutingRuleModelV2
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Routing rule to update: %+v", model))

	// Read the current routing rule from the Terraform state
	var currentModel RoutingRuleModelV2
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
	var model RoutingRuleModelV2
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

func upgradeRequestorV0(prior *RequestorModelV0) RequestorModelV1 {
	if prior.Type == "group" {
		return RequestorModelV1{
			Type: prior.Type,
			Groups: []GroupModelV1{{
				Directory: prior.Directory,
				Id:        prior.Id,
				Label:     prior.Label,
			}},
			Uid: prior.Uid,
		}
	}
	return RequestorModelV1{
		Type:   prior.Type,
		Groups: nil,
		Uid:    prior.Uid,
	}
}

func upgradeRequestorV1(prior *RequestorModelV1) RequestorModelV2 {
	if prior.Type == "group" {
		keepStr := "keep"
		return RequestorModelV2{
			Type:   prior.Type,
			Groups: prior.Groups,
			Uid:    prior.Uid,
			Effect: &keepStr,
		}
	}
	return RequestorModelV2{
		Type:   prior.Type,
		Groups: nil,
		Uid:    prior.Uid,
		Effect: nil,
	}
}

func upgradeApprovalV0(prior []ApprovalModelV0) []ApprovalModelV1 {
	upgraded := make([]ApprovalModelV1, len(prior))
	for i, approvalV0 := range prior {
		if approvalV0.Type == "group" {
			upgraded[i] = ApprovalModelV1{
				Directory:       approvalV0.Directory,
				Integration:     approvalV0.Integration,
				Groups:          []GroupModelV1{{Directory: approvalV0.Directory, Id: approvalV0.Id, Label: approvalV0.Label}},
				ProfileProperty: approvalV0.ProfileProperty,
				Options:         approvalV0.Options,
				Services:        approvalV0.Services,
				Type:            approvalV0.Type,
			}
			continue
		}
		upgraded[i] = ApprovalModelV1{
			Directory:       approvalV0.Directory,
			Integration:     approvalV0.Integration,
			Groups:          nil,
			ProfileProperty: approvalV0.ProfileProperty,
			Options:         approvalV0.Options,
			Services:        approvalV0.Services,
			Type:            approvalV0.Type,
		}
	}
	return upgraded
}

func upgradeApprovalV1(prior []ApprovalModelV1) []ApprovalModelV2 {
	upgraded := make([]ApprovalModelV2, len(prior))
	for i, approvalV1 := range prior {
		if approvalV1.Type == "group" {
			keepStr := "keep"
			upgraded[i] = ApprovalModelV2{
				Directory:       approvalV1.Directory,
				Integration:     approvalV1.Integration,
				Groups:          approvalV1.Groups,
				ProfileProperty: approvalV1.ProfileProperty,
				Options:         approvalV1.Options,
				Services:        approvalV1.Services,
				Type:            approvalV1.Type,
				Effect:          &keepStr,
			}
			continue
		}
		upgraded[i] = ApprovalModelV2{
			Directory:       approvalV1.Directory,
			Integration:     approvalV1.Integration,
			Groups:          nil,
			ProfileProperty: approvalV1.ProfileProperty,
			Options:         approvalV1.Options,
			Services:        approvalV1.Services,
			Type:            approvalV1.Type,
			Effect:          nil,
		}
	}
	return upgraded
}

func (rule *RoutingRule) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	var schemaV0 = newSingleRuleSchema(0)
	var schemaV1 = newSingleRuleSchema(1)
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schemaV0,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior RoutingRuleModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				requestor := upgradeRequestorV0(prior.Requestor)
				upgraded := RoutingRuleModelV1{
					Name:      prior.Name,
					Requestor: &requestor,
					Resource:  prior.Resource,
					Approval:  upgradeApprovalV0(prior.Approval),
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgraded)...)
			},
		},
		1: {
			PriorSchema: &schemaV1,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior RoutingRuleModelV1
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				requestor := upgradeRequestorV1(prior.Requestor)
				upgraded := RoutingRuleModelV2{
					Name:      prior.Name,
					Requestor: &requestor,
					Resource:  prior.Resource,
					Approval:  upgradeApprovalV1(prior.Approval),
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgraded)...)
			},
		},
	}
}
