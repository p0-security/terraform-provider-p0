// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

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
var _ resource.Resource = &AccessPolicy{}
var _ resource.ResourceWithImportState = &AccessPolicy{}
var _ resource.ResourceWithUpgradeState = &AccessPolicy{}
var _ resource.ResourceWithMoveState = &AccessPolicy{}

type AccessPolicy struct {
	data *internal.P0ProviderData
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type AccessPolicyJson struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Disabled  *bool             `json:"disabled,omitempty" tfsdk:"disabled"`
	Requestor RequestorJson     `json:"requestor" tfsdk:"requestor"`
	Resource  ResourceModel     `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV2 `json:"approval" tfsdk:"approval"`
}

// IdpGroupsJson mirrors the P0 app's `IdpGroups` wire shape: a `groups`+
// `effect` pair nested as a single field's value, rather than flattened
// into the parent (as it is everywhere else in this schema).
type IdpGroupsJson struct {
	Type   string         `json:"type"`
	Groups []GroupModelV1 `json:"groups"`
	Effect *string        `json:"effect"`
}

// AgentJson is the wire shape of an `agentic` requestor rule's `agent`
// sub-rule. Only the "owner-group" variant differs from AgentModel: its
// flat Groups/Effect fields are wrapped into a nested IdpGroupsJson object.
type AgentJson struct {
	Type           string         `json:"type"`
	ClientId       *string        `json:"clientId,omitempty"`
	Owner          *string        `json:"owner,omitempty"`
	Groups         *IdpGroupsJson `json:"groups,omitempty"`
	ProviderId     *string        `json:"providerId,omitempty"`
	SubjectPattern *string        `json:"subjectPattern,omitempty"`
}

type RequestorJson struct {
	Type   string            `json:"type"`
	Groups []GroupModelV1    `json:"groups,omitempty"`
	Uid    *string           `json:"uid,omitempty"`
	Effect *string           `json:"effect,omitempty"`
	Agent  *AgentJson        `json:"agent,omitempty"`
	User   *AgenticUserModel `json:"user,omitempty"`
}

// agentToJson wraps AgentModel's flat Groups/Effect into a nested
// IdpGroupsJson object, but only for the "owner-group" variant.
func agentToJson(model *AgentModel) *AgentJson {
	if model == nil {
		return nil
	}
	agent := &AgentJson{
		Type:           model.Type,
		ClientId:       model.ClientId,
		Owner:          model.Owner,
		ProviderId:     model.ProviderId,
		SubjectPattern: model.SubjectPattern,
	}
	if model.Type == "owner-group" {
		agent.Groups = &IdpGroupsJson{Type: "group", Groups: model.Groups, Effect: model.Effect}
	}
	return agent
}

// agentFromJson unwraps AgentJson's nested IdpGroupsJson (present only for
// the "owner-group" variant) back into AgentModel's flat Groups/Effect.
func agentFromJson(json *AgentJson) *AgentModel {
	if json == nil {
		return nil
	}
	agent := &AgentModel{
		Type:           json.Type,
		ClientId:       json.ClientId,
		Owner:          json.Owner,
		ProviderId:     json.ProviderId,
		SubjectPattern: json.SubjectPattern,
	}
	if json.Groups != nil {
		agent.Groups = json.Groups.Groups
		agent.Effect = json.Groups.Effect
	}
	return agent
}

func requestorToJson(model *RequestorModelV3) RequestorJson {
	return RequestorJson{
		Type:   model.Type,
		Groups: model.Groups,
		Uid:    model.Uid,
		Effect: model.Effect,
		Agent:  agentToJson(model.Agent),
		User:   model.User,
	}
}

func requestorFromJson(json RequestorJson) *RequestorModelV3 {
	return &RequestorModelV3{
		Type:   json.Type,
		Groups: json.Groups,
		Uid:    json.Uid,
		Effect: json.Effect,
		Agent:  agentFromJson(json.Agent),
		User:   json.User,
	}
}

func NewAccessPolicy() resource.Resource {
	return &AccessPolicy{}
}

func getPath(name string) string {
	encodedName := url.PathEscape(name)
	return fmt.Sprintf("policy/name/%s", encodedName)
}

func toJson(model AccessPolicyModelV3) AccessPolicyJson {
	return AccessPolicyJson{
		Name:      model.Name,
		Disabled:  model.Disabled,
		Requestor: requestorToJson(model.Requestor),
		Resource:  *model.Resource,
		Approval:  model.Approval}
}

func toModel(json AccessPolicyJson) AccessPolicyModelV3 {
	return AccessPolicyModelV3{
		Name:      json.Name,
		Disabled:  json.Disabled,
		Requestor: requestorFromJson(json.Requestor),
		Resource:  &json.Resource,
		Approval:  json.Approval,
	}
}

func newAccessPolicySchema(version int64) schema.Schema {
	attributes := map[string]schema.Attribute{
		"name": schema.StringAttribute{
			MarkdownDescription: "The name of the policy",
			Required:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"requestor": requestorAttribute(version),
		"resource":  resourceAttribute,
		"approval":  approvalAttribute(version),
	}
	// The disabled attribute postdates schema versions 0 and 1; including it in
	// their schemas breaks decoding of prior states into the V0/V1 models during
	// state upgrades and moves.
	if version >= 2 {
		attributes["disabled"] = schema.BoolAttribute{
			MarkdownDescription: "Whether or not the access policy should be evaluated; if false or not defined, the policy will be evaluated",
			Optional:            true,
		}
	}
	return schema.Schema{
		Version: version,
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: `An access policy that controls who can request access to what, and access requirements.
See [the P0 access-policy docs](https://docs.p0.dev/just-in-time-access/request-routing).`,
		Attributes: attributes,
	}
}

func (policy *AccessPolicy) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_policy"
}

func (policy *AccessPolicy) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = newAccessPolicySchema(currentSchemaVersion)
}

func (policy *AccessPolicy) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		policy.data = data
	}
}

func (policy *AccessPolicy) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var diag = &resp.Diagnostics

	// Load the plan into the model
	var model AccessPolicyModelV3
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Access policy to create: %+v", model))

	json := toJson(model)

	// Create the access policy
	var updatedJson AccessPolicyJson
	_, postErr := policy.data.Post(getPath(*model.Name), &json, &updatedJson)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to create access policy:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Latest access policy: %+v", updatedJson))

	updatedModel := toModel(updatedJson)

	// Update the Terraform state to reflect the newly created access policy
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), updatedModel.Name)...)
	diag.Append(resp.State.Set(ctx, updatedModel)...)
}

func (policy *AccessPolicy) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var diag = &resp.Diagnostics

	// Load the state into the model
	var model AccessPolicyModelV3
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	// Read the access policy
	var json AccessPolicyJson
	httpResponse, httpErr := policy.data.Get(getPath(*model.Name), &json)
	if httpErr != nil {
		// Check if the error indicates that the resource was not found (404)
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			tflog.Debug(ctx, "Access policy not found (404), removing from state")
			// Remove the resource from state by calling RemoveResource.
			resp.State.RemoveResource(ctx)
			return
		}

		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to read access policy:\n%s", httpErr))
		return
	}

	model = toModel(json)

	// Update the Terraform state to match the access policy returned by the API
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), model.Name)...)
	diag.Append(resp.State.Set(ctx, model)...)

	tflog.Debug(ctx, fmt.Sprintf("Reading access policy: %+v", model))
}

func (policy *AccessPolicy) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var diag = &resp.Diagnostics

	// Load the plan into the model
	var model AccessPolicyModelV3
	diag.Append(req.Plan.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Access policy to update: %+v", model))

	// Read the current access policy from the Terraform state
	var currentModel AccessPolicyModelV3
	diag.Append(req.State.Get(ctx, &currentModel)...)
	if diag.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Current access policy state: %+v", currentModel))

	json := toJson(model)

	// Update the access policy
	var updatedJson AccessPolicyJson
	_, postErr := policy.data.Put(getPath(*model.Name), &json, &updatedJson)
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to update access policy:\n%s", postErr))
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated access policy: %+v", updatedJson))

	updatedModel := toModel(updatedJson)

	// Update the Terraform state to reflect the updated access policy
	diag.Append(resp.State.SetAttribute(ctx, path.Root("name"), updatedModel.Name)...)
	diag.Append(resp.State.Set(ctx, updatedModel)...)
}

func (policy *AccessPolicy) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var diag = &resp.Diagnostics

	// Load the state into the model
	var model AccessPolicyModelV3
	diag.Append(req.State.Get(ctx, &model)...)
	if diag.HasError() {
		return
	}

	// Delete the access policy
	_, postErr := policy.data.Delete(getPath(*model.Name))
	if postErr != nil {
		diag.AddError("Error communicating with P0", fmt.Sprintf("Unable to delete access policy:\n%s", postErr))
	}
}

func (policy *AccessPolicy) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

func upgradeModelV0(prior AccessPolicyModelV0) AccessPolicyModelV1 {
	requestor := upgradeRequestorV0(prior.Requestor)
	return AccessPolicyModelV1{
		Name:      prior.Name,
		Requestor: &requestor,
		Resource:  prior.Resource,
		Approval:  upgradeApprovalV0(prior.Approval),
	}
}

func upgradeModelV1(prior AccessPolicyModelV1) AccessPolicyModelV2 {
	requestor := upgradeRequestorV1(prior.Requestor)
	return AccessPolicyModelV2{
		Name:      prior.Name,
		Requestor: &requestor,
		Resource:  prior.Resource,
		Approval:  upgradeApprovalV1(prior.Approval),
	}
}

// upgradeRequestorV2 is a pure passthrough: the `agentic` requestor type is
// purely additive, so prior `any`/`group`/`user` requestors need no
// representation change, and `agent`/`user` simply come out unset.
func upgradeRequestorV2(prior *RequestorModelV2) RequestorModelV3 {
	return RequestorModelV3{
		Type:   prior.Type,
		Groups: prior.Groups,
		Uid:    prior.Uid,
		Effect: prior.Effect,
	}
}

func upgradeModelV2(prior AccessPolicyModelV2) AccessPolicyModelV3 {
	requestor := upgradeRequestorV2(prior.Requestor)
	return AccessPolicyModelV3{
		Name:      prior.Name,
		Disabled:  prior.Disabled,
		Requestor: &requestor,
		Resource:  prior.Resource,
		Approval:  prior.Approval,
	}
}

func (policy *AccessPolicy) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	var schemaV0 = newAccessPolicySchema(0)
	var schemaV1 = newAccessPolicySchema(1)
	var schemaV2 = newAccessPolicySchema(2)
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schemaV0,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior AccessPolicyModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradeModelV0(prior))...)
			},
		},
		1: {
			PriorSchema: &schemaV1,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior AccessPolicyModelV1
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradeModelV1(prior))...)
			},
		},
		2: {
			PriorSchema: &schemaV2,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior AccessPolicyModelV2
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradeModelV2(prior))...)
			},
		},
	}
}

// isRoutingRuleMoveRequest reports whether a MoveState request originates from
// the deprecated p0_routing_rule resource at the given schema version. The
// provider address is deliberately not checked: it varies across registry
// mirrors, forks, and local -plugin-dir development, while the type name and
// schema version already pin the state shape.
func isRoutingRuleMoveRequest(req resource.MoveStateRequest, version int64) bool {
	return req.SourceTypeName == "p0_routing_rule" &&
		req.SourceSchemaVersion == version
}

// MoveState enables `moved` blocks from the deprecated p0_routing_rule
// resource (at any of its schema versions) to p0_access_policy.
func (policy *AccessPolicy) MoveState(ctx context.Context) []resource.StateMover {
	var schemaV0 = newAccessPolicySchema(0)
	var schemaV1 = newAccessPolicySchema(1)
	var schemaV2 = newAccessPolicySchema(2)
	var schemaV3 = newAccessPolicySchema(3)
	return []resource.StateMover{
		{
			SourceSchema: &schemaV0,
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				if !isRoutingRuleMoveRequest(req, 0) || req.SourceState == nil {
					return
				}
				var prior AccessPolicyModelV0
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.TargetState.Set(ctx, upgradeModelV2(upgradeModelV1(upgradeModelV0(prior))))...)
			},
		},
		{
			SourceSchema: &schemaV1,
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				if !isRoutingRuleMoveRequest(req, 1) || req.SourceState == nil {
					return
				}
				var prior AccessPolicyModelV1
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.TargetState.Set(ctx, upgradeModelV2(upgradeModelV1(prior)))...)
			},
		},
		{
			SourceSchema: &schemaV2,
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				if !isRoutingRuleMoveRequest(req, 2) || req.SourceState == nil {
					return
				}
				var prior AccessPolicyModelV2
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.TargetState.Set(ctx, upgradeModelV2(prior))...)
			},
		},
		{
			SourceSchema: &schemaV3,
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				if !isRoutingRuleMoveRequest(req, 3) || req.SourceState == nil {
					return
				}
				var prior AccessPolicyModelV3
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.TargetState.Set(ctx, prior)...)
			},
		},
	}
}
