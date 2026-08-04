// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type GroupModelV1 struct {
	Directory *string `json:"directory" tfsdk:"directory"`
	Id        *string `json:"id" tfsdk:"id"`
	Label     *string `json:"label" tfsdk:"label"`
}

type RequestorModelV0 struct {
	Directory *string `json:"directory" tfsdk:"directory"`
	Id        *string `json:"id" tfsdk:"id"`
	Label     *string `json:"label" tfsdk:"label"`
	Type      string  `json:"type" tfsdk:"type"`
	Uid       *string `json:"uid" tfsdk:"uid"`
}

type RequestorModelV1 struct {
	Type   string         `json:"type" tfsdk:"type"`
	Groups []GroupModelV1 `json:"groups" tfsdk:"groups"`
	Uid    *string        `json:"uid" tfsdk:"uid"`
}

type RequestorModelV2 struct {
	Type   string         `json:"type" tfsdk:"type"`
	Groups []GroupModelV1 `json:"groups" tfsdk:"groups"`
	Uid    *string        `json:"uid" tfsdk:"uid"`
	Effect *string        `json:"effect" tfsdk:"effect"`
}

// AgentModel is the TF-facing shape of an `agentic` requestor rule's `agent`
// sub-rule. `Groups`/`Effect` are kept flat (siblings of `Type`), matching
// every other groups+effect usage in this schema, even though the wire
// format nests them under a `groups: IdpGroups` object for the "owner-group"
// variant — see agentToJson/agentFromJson in access_policy.go.
type AgentModel struct {
	Type           string         `tfsdk:"type"`
	ClientId       *string        `tfsdk:"client_id"`
	Owner          *string        `tfsdk:"owner"`
	Groups         []GroupModelV1 `tfsdk:"groups"`
	Effect         *string        `tfsdk:"effect"`
	ProviderId     *string        `tfsdk:"provider_id"`
	SubjectPattern *string        `tfsdk:"subject_pattern"`
}

// AgenticUserModel is the shape of an `agentic` requestor rule's `user`
// sub-rule. It's flat on the wire (same as the outer requestor), so it needs
// no JSON conversion.
type AgenticUserModel struct {
	Type   string         `json:"type" tfsdk:"type"`
	Uid    *string        `json:"uid,omitempty" tfsdk:"uid"`
	Groups []GroupModelV1 `json:"groups,omitempty" tfsdk:"groups"`
	Effect *string        `json:"effect,omitempty" tfsdk:"effect"`
}

type RequestorModelV3 struct {
	Type   string            `json:"type" tfsdk:"type"`
	Groups []GroupModelV1    `json:"groups,omitempty" tfsdk:"groups"`
	Uid    *string           `json:"uid,omitempty" tfsdk:"uid"`
	Effect *string           `json:"effect,omitempty" tfsdk:"effect"`
	Agent  *AgentModel       `tfsdk:"agent"`
	User   *AgenticUserModel `json:"user,omitempty" tfsdk:"user"`
}

type ResourceFilterModel struct {
	Effect  string  `json:"effect" tfsdk:"effect"`
	Key     *string `json:"key" tfsdk:"key"`
	Pattern *string `json:"pattern" tfsdk:"pattern"`
	Value   *bool   `json:"value" tfsdk:"value"`
}

type ResourceModel struct {
	Type       string                          `json:"type" tfsdk:"type"`
	Service    *string                         `json:"service" tfsdk:"service"`
	AccessType *string                         `json:"accessType" tfsdk:"access_type"`
	Filters    *map[string]ResourceFilterModel `json:"filters" tfsdk:"filters"`
}

type ApprovalOptionsModel struct {
	AllowOneParty      *bool `json:"allowOneParty" tfsdk:"allow_one_party"`
	BreakGlassApprover *bool `json:"breakGlassApprover" tfsdk:"break_glass_approver"`
	RequirePreapproval *bool `json:"requirePreapproval" tfsdk:"require_preapproval"`
	RequireReason      *bool `json:"requireReason" tfsdk:"require_reason"`
	RequireDuration    *bool `json:"requireDuration" tfsdk:"require_duration"`
}

type ApprovalModelV0 struct {
	Directory       *string               `json:"directory" tfsdk:"directory"`
	Id              *string               `json:"id" tfsdk:"id"`
	Integration     *string               `json:"integration" tfsdk:"integration"`
	Label           *string               `json:"label" tfsdk:"label"`
	ProfileProperty *string               `json:"profileProperty" tfsdk:"profile_property"`
	Options         *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services        *[]string             `json:"services" tfsdk:"services"`
	Type            string                `json:"type" tfsdk:"type"`
}

type ApprovalModelV1 struct {
	Directory       *string               `json:"directory" tfsdk:"directory"`
	Integration     *string               `json:"integration" tfsdk:"integration"`
	Groups          []GroupModelV1        `json:"groups" tfsdk:"groups"`
	ProfileProperty *string               `json:"profileProperty" tfsdk:"profile_property"`
	Options         *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services        *[]string             `json:"services" tfsdk:"services"`
	Type            string                `json:"type" tfsdk:"type"`
}

type ApprovalModelV2 struct {
	Directory       *string               `json:"directory" tfsdk:"directory"`
	Integration     *string               `json:"integration" tfsdk:"integration"`
	Groups          []GroupModelV1        `json:"groups" tfsdk:"groups"`
	ProfileProperty *string               `json:"profileProperty" tfsdk:"profile_property"`
	Options         *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services        *[]string             `json:"services" tfsdk:"services"`
	Type            string                `json:"type" tfsdk:"type"`
	Effect          *string               `json:"effect" tfsdk:"effect"`
}

type AccessPolicyModelV0 struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Requestor *RequestorModelV0 `json:"requestor" tfsdk:"requestor"`
	Resource  *ResourceModel    `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV0 `json:"approval" tfsdk:"approval"`
}

type AccessPolicyModelV1 struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Requestor *RequestorModelV1 `json:"requestor" tfsdk:"requestor"`
	Resource  *ResourceModel    `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV1 `json:"approval" tfsdk:"approval"`
}

type AccessPolicyModelV2 struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Disabled  *bool             `json:"disabled,omitempty" tfsdk:"disabled"`
	Requestor *RequestorModelV2 `json:"requestor" tfsdk:"requestor"`
	Resource  *ResourceModel    `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV2 `json:"approval" tfsdk:"approval"`
}

type AccessPolicyModelV3 struct {
	Name      *string           `json:"name" tfsdk:"name"`
	Disabled  *bool             `json:"disabled,omitempty" tfsdk:"disabled"`
	Requestor *RequestorModelV3 `json:"requestor" tfsdk:"requestor"`
	Resource  *ResourceModel    `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModelV2 `json:"approval" tfsdk:"approval"`
}

const currentSchemaVersion int64 = 3

// requestorUnionAttributes builds the `type`/`uid`/`groups`/`effect`
// attributes shared by the top-level `requestor` object and the `agentic`
// requestor's nested `user` object — both are the same `any`/`group`/`user`
// union, just with `none` (headless agent, no fields) added for `user`.
func requestorUnionAttributes(version int64, typeDescription string) map[string]schema.Attribute {
	return AttachGroupFilterEffectAttribute(version, AttachGroupAttributes(version,
		map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: typeDescription,
				Required:            true,
			},
			"uid": schema.StringAttribute{MarkdownDescription: `Required, and may only be used, if 'type' is 'user'. This is the user's email address.`, Optional: true},
		}))
}

func requestorAttribute(version int64) schema.SingleNestedAttribute {
	attributes := requestorUnionAttributes(version, `How P0 matches requestors:
    - 'any': Any requestor will match
    - 'group': Members of a directory group will match
    - 'user': Only match a single user
    - 'agentic': Match agent sessions, based on the agent's identity and the human user (if any) behind it`)
	requirements := map[string][]string{
		"user":  {"uid"},
		"group": {"groups", "effect"},
	}
	if version >= currentSchemaVersion {
		attributes["agent"] = agentAttribute(version)
		attributes["user"] = agenticUserAttribute(version)
		requirements["agentic"] = []string{"agent", "user"}
	}
	attribute := schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: `Controls who has access. See [the Requestor docs](https://docs.p0.dev/just-in-time-access/request-routing#requestor).`,
		Attributes:          attributes,
	}
	// `groups` and `effect` only exist from schema version 2 onward, so only the
	// current schema can enforce their type-conditional requiredness.
	if version >= currentSchemaVersion {
		attribute.Validators = []validator.Object{
			RequiredWhenType(requirements),
		}
	}
	return attribute
}

// agentAttribute builds the `requestor.agent` schema: a discriminated union
// describing the agent's own identity, used only when `requestor.type` is
// 'agentic'.
func agentAttribute(version int64) schema.SingleNestedAttribute {
	attributes := AttachGroupFilterEffectAttribute(version, AttachGroupAttributes(version,
		map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `How P0 matches the agent:
    - 'any': Any agent will match
    - 'mcp-client': Only agents connecting through a specific MCP client will match
    - 'agent-owner': Only an agent owned by a specific user will match
    - 'owner-group': Only an agent owned by a member of a directory group will match
    - 'provider': Only an agent federated by a specific identity provider will match`,
				Required: true,
			},
			"client_id": schema.StringAttribute{MarkdownDescription: `Required, and may only be used, if 'type' is 'mcp-client'. The MCP client's identifier.`, Optional: true},
			"owner":     schema.StringAttribute{MarkdownDescription: `Required, and may only be used, if 'type' is 'agent-owner'. The agent owner's email address.`, Optional: true},
			"provider_id": schema.StringAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'provider'. The identifier of an installed identity-provider integration.`,
				Optional:            true,
			},
			"subject_pattern": schema.StringAttribute{
				MarkdownDescription: `May only be used if 'type' is 'provider'. An optional regular expression used to further narrow the agent's subject claim.`,
				Optional:            true,
			},
		}))
	// AttachGroupAttributes/AttachGroupFilterEffectAttribute's descriptions
	// assume the discriminator value is 'group'; here it's 'owner-group'.
	if groups, ok := attributes["groups"].(schema.ListNestedAttribute); ok {
		groups.MarkdownDescription = `Required, and may only be used, if 'type' is 'owner-group'. If the agent's owner is a member of any of these groups, the rule will match.`
		attributes["groups"] = groups
	}
	if effect, ok := attributes["effect"].(schema.StringAttribute); ok {
		effect.MarkdownDescription = `Required, and may only be used, if 'type' is 'owner-group'. The filter effect. May be one of:
    - 'keep': Access rule only applies when the agent's owner is a member of any of the specified groups
    - 'remove': Access rule only applies when the agent's owner is _not_ a member of any of the specified groups`
		attributes["effect"] = effect
	}
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: `Required, and may only be used, if the requestor 'type' is 'agentic'. Describes the agent's own identity.`,
		Attributes:          attributes,
		Validators: []validator.Object{
			RequiredWhenType(map[string][]string{
				"mcp-client":  {"client_id"},
				"agent-owner": {"owner"},
				"owner-group": {"groups", "effect"},
				"provider":    {"provider_id"},
			}),
		},
	}
}

// agenticUserAttribute builds the `requestor.user` schema: a discriminated
// union describing the human (if any) behind an agentic session, used only
// when `requestor.type` is 'agentic'.
func agenticUserAttribute(version int64) schema.SingleNestedAttribute {
	attributes := requestorUnionAttributes(version, `How P0 matches the human user behind the agent:
    - 'any': Any user, or no user, will match
    - 'group': Members of a directory group will match
    - 'user': Only match a single user
    - 'none': Only match a headless agent session with no human user`)
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: `Required, and may only be used, if the requestor 'type' is 'agentic'. Describes the human user (if any) behind the agent.`,
		Attributes:          attributes,
		Validators: []validator.Object{
			RequiredWhenType(map[string][]string{
				"user":  {"uid"},
				"group": {"groups", "effect"},
			}),
		},
	}
}

var resourceAttribute = schema.SingleNestedAttribute{
	Required:            true,
	MarkdownDescription: `Controls what is accessed. See [the Resource docs](https://docs.p0.dev/just-in-time-access/request-routing#resource).`,
	Validators: []validator.Object{
		RequiredWhenType(map[string][]string{
			"integration": {"service"},
		}),
	},
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
    - 'removeAll': Access rule does not apply to any item with this filter key`,
						Required: true,
					},
					"key": schema.StringAttribute{
						MarkdownDescription: `The value being filtered. Required if the filter effect is 'keep' or 'remove'.
See [docs](https://docs.p0.dev/just-in-time-access/request-routing#resource) for available values.`,
						Optional: true,
					},
					"value": schema.BoolAttribute{
						MarkdownDescription: `The value being filtered. Required if it's a boolean filter.`,
						Optional:            true,
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
			MarkdownDescription: `Required, and may only be used, if 'type' is 'integration'.
See [the Resource docs](https://docs.p0.dev/just-in-time-access/request-routing#resource) for a list of available services.`,
			Optional: true,
		},
		"type": schema.StringAttribute{
			MarkdownDescription: `How P0 matches resources:
    - 'any': Any resource
    - 'integration': Only resources within a specified integration`,
			Required: true,
		},
		"access_type": schema.StringAttribute{
			MarkdownDescription: `May only be used if 'type' is 'integration' and must be a valid access type for a given service integration or 'any'. Defaults to 'any' if not specified.`,
			Optional:            true,
		},
	},
}

func approvalAttribute(version int64) schema.ListNestedAttribute {
	nestedObject := schema.NestedAttributeObject{
		Attributes: AttachGroupFilterEffectAttribute(version, AttachGroupAttributes(version, map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'requestor-profile'. One of "azure-ad", "entra-id", "okta", or "workspace".`,
				Optional:            true,
			},
			"integration": schema.StringAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'auto' or 'escalation'. Possible values:
- 'pagerduty': Access is granted if the requestor is on-call in PagerDuty.
- 'incidentio': Access is granted if the requestor is on-call in incident.io.`,
				Optional: true,
			},
			"options": schema.SingleNestedAttribute{
				MarkdownDescription: `If present, determines additional trust requirements.`,
				Attributes: map[string]schema.Attribute{
					"allow_one_party": schema.BoolAttribute{
						MarkdownDescription: `If true, allows requestors to approve their own requests. Does not apply to 'auto' approval rules.`,
						Optional:            true,
					},
					"require_reason": schema.BoolAttribute{
						MarkdownDescription: `If true, requires access requests to include a reason.`,
						Optional:            true,
					},
					"require_duration": schema.BoolAttribute{
						MarkdownDescription: `If true, requires access requests to include a duration.`,
						Optional:            true,
					},
					"require_preapproval": schema.BoolAttribute{
						MarkdownDescription: `If true, requires access requests to be pre-approved.`,
						Optional:            true,
					},
					"break_glass_approver": schema.BoolAttribute{
						MarkdownDescription: `If true, allows the approver to approve break-glass requests. Does not apply to 'auto' approval rules.`,
						Optional:            true,
					},
				},
				Optional: true,
			},
			"profile_property": schema.StringAttribute{
				MarkdownDescription: `May only be used if 'type' is 'requestor-profile'. This is the profile attribute that contains the manager's email.`,
				Optional:            true,
			},
			"services": schema.ListAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'escalation'. Defines which services to page on escalation.`,
				ElementType:         types.StringType,
				Optional:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: `Determines trust requirements for access. If empty, access is disallowed. Except for 'deny', meeting any requirement is sufficient to grant access. Possible values:
    - 'auto': Access is granted according to the requirements of the specified 'integration'
    - 'deny': Access is always denied
    - 'escalation': Access may be approved by on-call members of the specified services, who are paged when access is manually escalated by the requestor
    - 'group': Access may be granted by any member of the defined directory group
    - 'persistent': Access is always granted
    - 'requestor-profile': Allows approval by a user specified by a field in the requestor's IDP profile
    - 'p0': Access may be granted by any user with the P0 "security reviewer" role (defined in the P0 app)`,
				Required: true,
			},
		})),
	}
	// `groups` and `effect` only exist from schema version 2 onward, so only the
	// current schema can enforce their type-conditional requiredness.
	if version >= currentSchemaVersion {
		nestedObject.Validators = []validator.Object{
			RequiredWhenType(map[string][]string{
				"auto":              {"integration"},
				"escalation":        {"integration", "services"},
				"group":             {"groups", "effect"},
				"requestor-profile": {"directory"},
			}),
		}
	}
	return schema.ListNestedAttribute{
		MarkdownDescription: `Determines access requirements. See [the Approval docs](https://docs.p0.dev/just-in-time-access/request-routing#approval).`,
		Required:            true,
		NestedObject:        nestedObject,
	}
}
