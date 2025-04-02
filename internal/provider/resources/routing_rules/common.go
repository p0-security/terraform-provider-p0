// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package routingrules

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type GroupModel struct {
	Directory *string `json:"directory" tfsdk:"directory"`
	Id        *string `json:"id" tfsdk:"id"`
	Label     *string `json:"label" tfsdk:"label"`
}

type RequestorModel struct {
	Type   string       `json:"type" tfsdk:"type"`
	Groups []GroupModel `json:"groups" tfsdk:"groups"`
	Uid    *string      `json:"uid" tfsdk:"uid"`
}

type ResourceFilterModel struct {
	Effect  string  `json:"effect" tfsdk:"effect"`
	Key     *string `json:"key" tfsdk:"key"`
	Pattern *string `json:"pattern" tfsdk:"pattern"`
	Value   *bool   `json:"value" tfsdk:"value"`
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
	Directory       *string               `json:"directory" tfsdk:"directory"`
	Integration     *string               `json:"integration" tfsdk:"integration"`
	Groups          []GroupModel          `json:"groups" tfsdk:"groups"`
	ProfileProperty *string               `json:"profileProperty" tfsdk:"profile_property"`
	Options         *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services        *[]string             `json:"services" tfsdk:"services"`
	Type            string                `json:"type" tfsdk:"type"`
}

type RoutingRuleModel struct {
	Name      *string         `json:"name" tfsdk:"name"`
	Requestor *RequestorModel `json:"requestor" tfsdk:"requestor"`
	Resource  *ResourceModel  `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModel `json:"approval" tfsdk:"approval"`
}

const currentSchemaVersion int64 = 1

var False = false

func requestorAttribute(version int64) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: `Controls who has access. See [the Requestor docs](https://docs.p0.dev/just-in-time-access/request-routing#requestor).`,
		Attributes: AttachGroupAttributes(version,
			map[string]schema.Attribute{
				"type": schema.StringAttribute{
					MarkdownDescription: `How P0 matches requestors:
			- 'any': Any requestor will match
			- 'group': Members of a directory group will match
			- 'user': Only match a single user`,
					Required: true,
				},
				"uid": schema.StringAttribute{MarkdownDescription: `May only be used if 'type' is 'user'. This is the user's email address.`, Optional: true},
			}),
	}
}

var resourceAttribute = schema.SingleNestedAttribute{
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
}

func approvalAttribute(version int64) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: `Determines access requirements. See [the Approval docs](https://docs.p0.dev/just-in-time-access/request-routing#approval).`,
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: AttachGroupAttributes(version, map[string]schema.Attribute{
				"directory": schema.StringAttribute{
					MarkdownDescription: `May only be used if 'type' is 'requestor-profile'. One of "azure-ad", "okta", or "workspace".`,
					Optional:            true,
				},
				"integration": schema.StringAttribute{
					MarkdownDescription: `May only be used if 'type' is 'auto' or 'escalation'. Possible values:
- 'pagerduty': Access is granted if the requestor is on-call.`,
					Optional: true,
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
				"profile_property": schema.StringAttribute{
					MarkdownDescription: `May only be used if 'type' is 'requestor-profile'. This is the profile attribute that contains the manager's email.`,
					Optional:            true,
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
    - 'escalation': Access may be approved by on-call members of the specified services, who are paged when access is manually escalated by the requestor
    - 'group': Access may be granted by any member of the defined directory group
    - 'persistent': Access is always granted
    - 'requestor-profile': Allows approval by a user specified by a field in the requestor's IDP profile
    - 'p0': Access may be granted by any user with the P0 "approver" role (defined in the P0 app)`,
					Required: true,
				},
			}),
		},
	}
}
