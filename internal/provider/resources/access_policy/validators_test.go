// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requestorRequirements mirrors the type-conditional requirements enforced on
// the requestor object (see requestorAttribute).
var requestorRequirements = map[string][]string{
	"user":  {"uid"},
	"group": {"groups", "effect"},
}

var requestorAttrTypes = map[string]attr.Type{
	"type":   types.StringType,
	"uid":    types.StringType,
	"groups": types.StringType, // stand-in; the validator only checks null-ness
	"effect": types.StringType,
}

// TestRequiredWhenType verifies that attributes are required for the `type`
// values that list them, and ignored otherwise, mirroring the `required`
// arrays in the P0 app's shared/src/types/policy/types.json.
func TestRequiredWhenType(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{
			name:    "user without uid errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": null, "groups": null, "effect": null},
			wantErr: true,
		},
		{
			name:    "user with uid passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": set, "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "group missing effect errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": null},
			wantErr: true,
		},
		{
			name:    "group with groups and effect passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": set},
			wantErr: false,
		},
		{
			name:    "type without requirements passes despite null attrs",
			attrs:   map[string]attr.Value{"type": types.StringValue("any"), "uid": null, "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "unknown uid is allowed (deferred to apply)",
			attrs:   map[string]attr.Value{"type": types.StringValue("user"), "uid": types.StringUnknown(), "groups": null, "effect": null},
			wantErr: false,
		},
		{
			name:    "null type is skipped",
			attrs:   map[string]attr.Value{"type": null, "uid": null, "groups": null, "effect": null},
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(requestorAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenType(requestorRequirements).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestRequiredWhenTypeErrorPath verifies the diagnostic is anchored to the
// specific missing attribute (e.g. requestor.uid) rather than the parent object.
func TestRequiredWhenTypeErrorPath(t *testing.T) {
	object, diags := types.ObjectValue(requestorAttrTypes, map[string]attr.Value{
		"type": types.StringValue("user"), "uid": types.StringNull(), "groups": types.StringNull(), "effect": types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("failed to build object: %v", diags)
	}

	resp := &validator.ObjectResponse{}
	RequiredWhenType(requestorRequirements).ValidateObject(
		context.Background(),
		validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: object},
		resp,
	)

	if len(resp.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(resp.Diagnostics), resp.Diagnostics)
	}
	withPath, ok := resp.Diagnostics[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("expected a diagnostic with a path, got %T", resp.Diagnostics[0])
	}
	if got, want := withPath.Path().String(), "requestor.uid"; got != want {
		t.Errorf("diagnostic path = %q; want %q", got, want)
	}
}

// TestRequiredWhenTypeNullObject verifies a null object is skipped entirely.
func TestRequiredWhenTypeNullObject(t *testing.T) {
	resp := &validator.ObjectResponse{}
	RequiredWhenType(requestorRequirements).ValidateObject(
		context.Background(),
		validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: types.ObjectNull(requestorAttrTypes)},
		resp,
	)
	if resp.Diagnostics.HasError() {
		t.Errorf("null object should not error: %v", resp.Diagnostics)
	}
}

// agentRequirements mirrors the type-conditional requirements enforced on
// the `requestor.agent` object (see agentAttribute).
var agentRequirements = map[string][]string{
	"agent-client": {"client_id"},
	// Deprecated alias for "agent-client"; the API accepts both.
	"mcp-client":  {"client_id"},
	"agent-owner": {"owner"},
	"owner-group": {"groups", "effect"},
	"provider":    {"provider_id"},
}

var agentAttrTypes = map[string]attr.Type{
	"type":            types.StringType,
	"client_id":       types.StringType,
	"owner":           types.StringType,
	"groups":          types.StringType, // stand-in; the validator only checks null-ness
	"effect":          types.StringType,
	"provider_id":     types.StringType,
	"subject_pattern": types.StringType,
}

// TestRequiredWhenTypeAgent verifies the type-conditional requirements for
// each `requestor.agent` variant, mirroring the `required` arrays for
// AgentOwnerRule/AgentOwnerGroupRule/AgentProviderRule/McpClientRequestorRule
// in the P0 app's shared/src/types/policy/types.json.
func TestRequiredWhenTypeAgent(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()
	base := func(overrides map[string]attr.Value) map[string]attr.Value {
		attrs := map[string]attr.Value{
			"type": null, "client_id": null, "owner": null, "groups": null, "effect": null, "provider_id": null, "subject_pattern": null,
		}
		for k, v := range overrides {
			attrs[k] = v
		}
		return attrs
	}

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{"any needs nothing", base(map[string]attr.Value{"type": types.StringValue("any")}), false},
		{"mcp-client missing client_id errors", base(map[string]attr.Value{"type": types.StringValue("mcp-client")}), true},
		{"mcp-client with client_id passes", base(map[string]attr.Value{"type": types.StringValue("mcp-client"), "client_id": set}), false},
		{"agent-owner missing owner errors", base(map[string]attr.Value{"type": types.StringValue("agent-owner")}), true},
		{"agent-owner with owner passes", base(map[string]attr.Value{"type": types.StringValue("agent-owner"), "owner": set}), false},
		{"owner-group missing groups and effect errors", base(map[string]attr.Value{"type": types.StringValue("owner-group")}), true},
		{"owner-group with groups and effect passes", base(map[string]attr.Value{"type": types.StringValue("owner-group"), "groups": set, "effect": set}), false},
		{"provider missing provider_id errors", base(map[string]attr.Value{"type": types.StringValue("provider")}), true},
		{"provider with only provider_id passes (subject_pattern optional)", base(map[string]attr.Value{"type": types.StringValue("provider"), "provider_id": set}), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(agentAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenType(agentRequirements).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor").AtName("agent"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// agentExclusivity mirrors the `ExclusiveToType` map attached to
// `requestor.agent` (see agentAttribute): `groups`/`effect` are only
// forwarded to the API for the "owner-group" variant (see agentToJson), so
// setting them for any other type must fail at plan time instead of being
// silently dropped.
var agentExclusivity = map[string][]string{
	"owner-group": {"groups", "effect"},
}

// TestExclusiveToTypeAgent verifies `groups`/`effect` are rejected on
// `requestor.agent` for every type except "owner-group".
func TestExclusiveToTypeAgent(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()
	base := func(overrides map[string]attr.Value) map[string]attr.Value {
		attrs := map[string]attr.Value{
			"type": null, "client_id": null, "owner": null, "groups": null, "effect": null, "provider_id": null, "subject_pattern": null,
		}
		for k, v := range overrides {
			attrs[k] = v
		}
		return attrs
	}

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{"owner-group with groups and effect passes", base(map[string]attr.Value{"type": types.StringValue("owner-group"), "groups": set, "effect": set}), false},
		{"provider with groups errors", base(map[string]attr.Value{"type": types.StringValue("provider"), "provider_id": set, "groups": set}), true},
		{"provider with effect errors", base(map[string]attr.Value{"type": types.StringValue("provider"), "provider_id": set, "effect": set}), true},
		{"mcp-client with groups and effect errors", base(map[string]attr.Value{"type": types.StringValue("mcp-client"), "client_id": set, "groups": set, "effect": set}), true},
		{"any without groups or effect passes", base(map[string]attr.Value{"type": types.StringValue("any")}), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(agentAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			ExclusiveToType(agentExclusivity).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor").AtName("agent"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestRequiredWhenTypeAgenticUser verifies the type-conditional requirements
// for each `requestor.user` variant (an `agentic` requestor's human-user
// sub-rule), mirroring AnyRule/GroupRequestorRule/NoUserRule/UserRequestorRule.
func TestRequiredWhenTypeAgenticUser(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{"any needs nothing", map[string]attr.Value{"type": types.StringValue("any"), "uid": null, "groups": null, "effect": null}, false},
		{"none needs nothing", map[string]attr.Value{"type": types.StringValue("none"), "uid": null, "groups": null, "effect": null}, false},
		{"user without uid errors", map[string]attr.Value{"type": types.StringValue("user"), "uid": null, "groups": null, "effect": null}, true},
		{"user with uid passes", map[string]attr.Value{"type": types.StringValue("user"), "uid": set, "groups": null, "effect": null}, false},
		{"group missing effect errors", map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": null}, true},
		{"group with groups and effect passes", map[string]attr.Value{"type": types.StringValue("group"), "uid": null, "groups": set, "effect": set}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(requestorAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenType(requestorRequirements).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor").AtName("user"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestRequiredWhenTypeRequestorAgentic verifies the outer `requestor` object
// requires `agent`/`user` when `type` is 'agentic'.
func TestRequiredWhenTypeRequestorAgentic(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()
	requirements := map[string][]string{
		"user":    {"uid"},
		"group":   {"groups", "effect"},
		"agentic": {"agent", "user"},
	}
	attrTypes := map[string]attr.Type{
		"type":   types.StringType,
		"uid":    types.StringType,
		"groups": types.StringType,
		"effect": types.StringType,
		"agent":  types.StringType,
		"user":   types.StringType,
	}

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{
			name:    "agentic missing agent and user errors",
			attrs:   map[string]attr.Value{"type": types.StringValue("agentic"), "uid": null, "groups": null, "effect": null, "agent": null, "user": null},
			wantErr: true,
		},
		{
			name:    "agentic with agent and user passes",
			attrs:   map[string]attr.Value{"type": types.StringValue("agentic"), "uid": null, "groups": null, "effect": null, "agent": set, "user": set},
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(attrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			RequiredWhenType(requirements).ValidateObject(
				context.Background(),
				validator.ObjectRequest{Path: path.Root("requestor"), ConfigValue: object},
				resp,
			)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestAgentAttributeValidators runs the validators attached to the real
// `requestor.agent` schema (agentAttribute), so the requirement map in the
// production schema is under test rather than a mirrored copy. The backend
// accepts both "agent-client" and its deprecated alias "mcp-client" for the
// client-scoped variant (AgentClientRequestorRule in the P0 app's
// shared/src/types/policy/types.ts), and requires clientId for both.
func TestAgentAttributeValidators(t *testing.T) {
	set := types.StringValue("x")
	null := types.StringNull()
	base := func(overrides map[string]attr.Value) map[string]attr.Value {
		attrs := map[string]attr.Value{
			"type": null, "client_id": null, "owner": null, "groups": null, "effect": null, "provider_id": null, "subject_pattern": null,
		}
		for k, v := range overrides {
			attrs[k] = v
		}
		return attrs
	}

	cases := []struct {
		name    string
		attrs   map[string]attr.Value
		wantErr bool
	}{
		{"agent-client missing client_id errors", base(map[string]attr.Value{"type": types.StringValue("agent-client")}), true},
		{"agent-client with client_id passes", base(map[string]attr.Value{"type": types.StringValue("agent-client"), "client_id": set}), false},
		{"mcp-client (deprecated alias) missing client_id errors", base(map[string]attr.Value{"type": types.StringValue("mcp-client")}), true},
		{"mcp-client (deprecated alias) with client_id passes", base(map[string]attr.Value{"type": types.StringValue("mcp-client"), "client_id": set}), false},
	}

	attribute := agentAttribute(currentSchemaVersion)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			object, diags := types.ObjectValue(agentAttrTypes, c.attrs)
			if diags.HasError() {
				t.Fatalf("failed to build object: %v", diags)
			}
			resp := &validator.ObjectResponse{}
			for _, v := range attribute.Validators {
				v.ValidateObject(
					context.Background(),
					validator.ObjectRequest{Path: path.Root("requestor").AtName("agent"), ConfigValue: object},
					resp,
				)
			}
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("HasError() = %v; want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestAgentAttributeDocumentsAgentClient guards the schema documentation the
// registry docs are generated from: the primary client-scoped value is
// "agent-client"; "mcp-client" remains only as a deprecated alias.
func TestAgentAttributeDocumentsAgentClient(t *testing.T) {
	attribute := agentAttribute(currentSchemaVersion)

	typeAttr, ok := attribute.Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("agent `type` attribute is not a StringAttribute")
	}
	if !strings.Contains(typeAttr.MarkdownDescription, "'agent-client'") {
		t.Errorf("agent `type` description does not document 'agent-client':\n%s", typeAttr.MarkdownDescription)
	}
	if !strings.Contains(typeAttr.MarkdownDescription, "'mcp-client' is a deprecated alias") {
		t.Errorf("agent `type` description does not note the deprecated 'mcp-client' alias:\n%s", typeAttr.MarkdownDescription)
	}

	clientIdAttr, ok := attribute.Attributes["client_id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("agent `client_id` attribute is not a StringAttribute")
	}
	if !strings.Contains(clientIdAttr.MarkdownDescription, "'agent-client'") {
		t.Errorf("`client_id` description does not document 'agent-client':\n%s", clientIdAttr.MarkdownDescription)
	}
	if !strings.Contains(clientIdAttr.MarkdownDescription, "deprecated alias 'mcp-client'") {
		t.Errorf("`client_id` description does not note the deprecated 'mcp-client' alias:\n%s", clientIdAttr.MarkdownDescription)
	}
}
