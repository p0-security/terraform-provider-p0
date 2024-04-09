// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

type RequestorModel struct {
	Directory *string `json:"directory" tfsdk:"directory"`
	Id        *string `json:"id" tfsdk:"id"`
	Label     *string `json:"label" tfsdk:"label"`
	Type      string  `json:"type" tfsdk:"type"`
	Uid       *string `json:"uid" tfsdk:"uid"`
}

type ResourceFilterModel struct {
	Effect  string  `json:"effect" tfsdk:"effect"`
	Key     *string `json:"key" tfsdk:"key"`
	Pattern *string `json:"pattern" tfsdk:"pattern"`
}

type ResourceModel struct {
	Filters *map[string]ResourceFilterModel `json:"filters" tfsdk:"filters"`
	Service *string                         `json:"service" tfsdk:"service"`
	Type    string                          `json:"type" tfsdk:"type"`
}

type ApprovalOptionsModel struct {
	AllowOneParty *bool `json:"allowOneParty" tfsdk:"allow_one_party"`
	RequireReason *bool `json:"requireReason" tfsdk:"require_reason"`
}

type ApprovalModel struct {
	Directory   *string               `json:"directory" tfsdk:"directory"`
	Id          *string               `json:"id" tfsdk:"id"`
	Integration *string               `json:"integration" tfsdk:"integration"`
	Label       *string               `json:"label" tfsdk:"label"`
	Options     *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services    *[]string             `json:"services" tfsdk:"services"`
	Type        string                `json:"type" tfsdk:"type"`
}

type RoutingRuleModel struct {
	Requestor RequestorModel  `json:"requestor" tfsdk:"requestor"`
	Resource  ResourceModel   `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModel `json:"approval" tfsdk:"approval"`
}

type RoutingRulesModel struct {
	Rule    []RoutingRuleModel `tfsdk:"rule"`
	Version types.String       `tfsdk:"version"`
}

// Need a separate representation for JSON data as version handling is different:
// - In TF state, it may be present, unknown (during update), or null
// - In JSON state, it is either present or null.
type RoutingRulesRepr struct {
	Rule    []RoutingRuleModel `json:"rules"`
	Version *string            `json:"version"`
}

type WorkflowLatestApi struct {
	Workflow RoutingRulesRepr `json:"workflow"`
}

var False = false
var DefaultRoutingRules = RoutingRulesRepr{
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
			"rule": schema.SetNestedBlock{
				MarkdownDescription: "All access rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"requestor": schema.SingleNestedAttribute{
							Required:            true,
							MarkdownDescription: `Controls who has access. See [the Requestor docs](https://docs.p0.dev/just-in-time-access/request-routing#requestor).`,
							Attributes: map[string]schema.Attribute{
								"directory": schema.StringAttribute{
									MarkdownDescription: `May only be used if 'type' is 'group'. One of "azure-ad", "okta", or "workspace".`,
									Optional:            true},
								"id": schema.StringAttribute{
									MarkdownDescription: `May only be used if 'type' is 'group'. This is the directory's internal group identifier for matching requestors.`,
									Optional:            true},
								"label": schema.StringAttribute{
									MarkdownDescription: `May only be used if 'type' is 'group'. This is any human-readable name for the directory group specified in the 'id' attribute.`,
									Optional:            true},
								"type": schema.StringAttribute{
									MarkdownDescription: `How P0 matches requestors:
    - 'any': Any requestor will match
    - 'group': Members of a directory group will match
    - 'user': Only match a single user`,
									Required: true,
								},
								"uid": schema.StringAttribute{MarkdownDescription: `May only be used if 'type' is 'user'. This is the user's email address.`, Optional: true},
							},
						},
						"resource": schema.SingleNestedAttribute{
							Required:            true,
							MarkdownDescription: `Controls what is accessed. See [the Resource docs](https://docs.p0.dev/just-in-time-access/request-routing#resource).`,
							Attributes: map[string]schema.Attribute{
								"filters": schema.MapNestedAttribute{
									MarkdownDescription: `May only be used if 'type' is 'integration'. Available filters depend on the value of 'service'.
See [the Resource docs](https://docs.p0.dev/just-in-time-access/request-routing#resource) for a list of available filters.`,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"effect": schema.StringAttribute{
												MarkdownDescription: `The filter effect. May be one of:
		- 'keep': Access rule only applies to items matching this filter
		- 'remove': Access rule only applies to items _not_ matching this filter
		- 'removeAll': Access rule does not apply to any item with this filter key
`,
												Required: true,
											},
											"key": schema.StringAttribute{
												MarkdownDescription: `The value being filtered. Required if the filter effect is 'keep' or 'remove'.
See [docs](https://docs.p0.dev/just-in-time-access/request-routing#resource) for available values.`,
												Optional: true,
											},
											"pattern": schema.StringAttribute{
												MarkdownDescription: `Filter patterns. Patterns are unanchored.`,
												Optional:            true,
											},
										},
									},
									Optional: true,
								},
								"service": schema.StringAttribute{
									MarkdownDescription: `May only be used if 'type' is 'integration'.
See [the Resource docs](https://docs.p0.dev/just-in-time-access/request-routing#resource) for a list of available services.`,
									Optional: true,
								},
								"type": schema.StringAttribute{
									MarkdownDescription: `How P0 matches resources:
    - 'any': Any resource
    - 'integration': Only resources within a specified integration`,
									Required: true,
								},
							},
						},
					},
					Blocks: map[string]schema.Block{
						"approval": schema.ListNestedBlock{
							MarkdownDescription: `Determines access requirements. See [the Approval docs](https://docs.p0.dev/just-in-time-access/request-routing#approval).`,
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"directory": schema.StringAttribute{
										MarkdownDescription: `May only be used if 'type' is 'group'. One of "azure-ad", "okta", or "workspace".`,
										Optional:            true,
									},
									"id": schema.StringAttribute{
										MarkdownDescription: `May only be used if 'type' is 'group'. This is the directory's internal group identifier for matching approvers.`,
										Optional:            true,
									},
									"integration": schema.StringAttribute{
										MarkdownDescription: `May only be used if 'type' is 'auto' or 'escalation'. Possible values:
    - 'pagerduty': Access is granted if the requestor is on-call.`,
										Optional: true,
									},
									"label": schema.StringAttribute{
										MarkdownDescription: `May only be used if 'type' is 'group'. This is any human-readable name for the directory group specified in the 'id' attribute.`,
										Optional:            true,
									},
									"options": schema.SingleNestedAttribute{
										MarkdownDescription: `If present, determines additional trust requirements.`,
										Attributes: map[string]schema.Attribute{
											"allow_one_party": schema.BoolAttribute{
												MarkdownDescription: `If true, allows requestors to approve their own requests.`,
												Optional:            true,
											},
											"require_reason": schema.BoolAttribute{
												MarkdownDescription: `If true, requires access requests to include a reason.`,
												Optional:            true,
											},
										},
										Optional: true,
									},
									"services": schema.ListAttribute{
										MarkdownDescription: `May only be used if 'type' is 'escalation'. Defines which services to page on escalation.`,
										ElementType:         types.StringType,
										Optional:            true,
									},
									"type": schema.StringAttribute{
										MarkdownDescription: `Determines trust requirements for access. If empty, access is disallowed. Except for 'deny', meeting any requirement is sufficient to grant access. Possible values:
    - 'auto': Access is granted according to the requirements of the specified 'integration'
    - 'deny': Access is always denied
    - 'escalation': Access may be approved by on-call members of the specified services, who are paged when access is requested
    - 'group': Access may be granted by any member of the defined directory group
    - 'p0': Access may be granted by any user with the P0 approval role (defined in the P0 app)`,
										Required: true,
									},
								},
							},
						}},
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

func (r *RoutingRules) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(internal.P0ProviderData)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected internal.P0ProviderData, got: %T. Please report this issue to support@p0.dev.", req.ProviderData),
		)

		return
	}

	r.data = &data
}

// Updates TF state based on current state and P0 routing-rules API response.
func (r *RoutingRules) updateState(ctx context.Context, diags *diag.Diagnostics, state *tfsdk.State, data RoutingRulesModel, latest WorkflowLatestApi) {
	if latest.Workflow.Version == nil {
		diags.AddError("Missing routing rules version", "P0 did not return a version for routing rules; please report this to support@p0.dev.")
		return
	}

	data.Rule = latest.Workflow.Rule
	data.Version = types.StringValue(*latest.Workflow.Version)

	// Save updated data into Terraform state
	diags.Append(state.Set(ctx, data)...)
}

// Posts a new routing-rules version to P0. This is used for create, update, and delete. Note that delete does not delete, but
// rather posts a default routing-rules set.
func (r *RoutingRules) postVersion(ctx context.Context, data RoutingRulesModel, diag *diag.Diagnostics, state *tfsdk.State) {
	workflowUpdate := WorkflowLatestApi{Workflow: RoutingRulesRepr{Rule: data.Rule}}

	postBody, marshalErr := json.Marshal(&workflowUpdate)
	if marshalErr != nil {
		diag.AddError("Could not serialize routing rules state", fmt.Sprintf("Encountered error: %s", marshalErr))
		return
	}

	var latest WorkflowLatestApi
	postErr := r.data.Post("workflow", postBody, &latest)
	if postErr != nil {
		diag.AddError("Could not update routing rules", fmt.Sprintf("Encountered error: %s", postErr))
		return
	}

	r.updateState(ctx, diag, state, data, latest)
}

func (r *RoutingRules) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RoutingRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.postVersion(ctx, data, &resp.Diagnostics, &resp.State)
}

func (r *RoutingRules) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoutingRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var latest WorkflowLatestApi
	httpErr := r.data.Get("workflow/latest", &latest)
	if httpErr != nil {
		resp.Diagnostics.AddError("Error communicationg with P0", fmt.Sprintf("Unable to read routing rules, got error:\n%s", httpErr))
		return
	}

	r.updateState(ctx, &resp.Diagnostics, &resp.State, data, latest)
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
	data.Rule = DefaultRoutingRules.Rule

	r.postVersion(ctx, data, &resp.Diagnostics, &resp.State)
}

func (r *RoutingRules) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("version"), req, resp)
}
