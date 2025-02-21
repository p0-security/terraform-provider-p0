// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package routingrules

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
	Id              *string               `json:"id" tfsdk:"id"`
	Integration     *string               `json:"integration" tfsdk:"integration"`
	Label           *string               `json:"label" tfsdk:"label"`
	ProfileProperty *string               `json:"profileProperty" tfsdk:"profile_property"`
	Options         *ApprovalOptionsModel `json:"options" tfsdk:"options"`
	Services        *[]string             `json:"services" tfsdk:"services"`
	Type            string                `json:"type" tfsdk:"type"`
}

type RoutingRuleModel struct {
	Requestor RequestorModel  `json:"requestor" tfsdk:"requestor"`
	Resource  ResourceModel   `json:"resource" tfsdk:"resource"`
	Approval  []ApprovalModel `json:"approval" tfsdk:"approval"`
}

var False = false
