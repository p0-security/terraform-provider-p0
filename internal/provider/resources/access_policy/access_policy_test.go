// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal/provider/resources/duration"
)

// TestAgentToJsonOwnerGroup verifies the "owner-group" variant's flat
// Groups/Effect fields get wrapped into a nested IdpGroupsJson object with
// type "group", matching the P0 app's IdpGroups wire shape.
func TestAgentToJsonOwnerGroup(t *testing.T) {
	effect := "keep"
	model := &AgentModel{
		Type:   "owner-group",
		Groups: []GroupModelV1{{Directory: strPtr("okta"), Id: strPtr("123"), Label: strPtr("Admins")}},
		Effect: &effect,
	}

	got := agentToJson(model)

	if got.Type != "owner-group" {
		t.Fatalf("Type = %q; want %q", got.Type, "owner-group")
	}
	if got.Groups == nil {
		t.Fatalf("Groups is nil; want a wrapped IdpGroupsJson")
	}
	if got.Groups.Type != "group" {
		t.Errorf("Groups.Type = %q; want %q", got.Groups.Type, "group")
	}
	if !reflect.DeepEqual(got.Groups.Groups, model.Groups) {
		t.Errorf("Groups.Groups = %+v; want %+v", got.Groups.Groups, model.Groups)
	}
	if got.Groups.Effect == nil || *got.Groups.Effect != effect {
		t.Errorf("Groups.Effect = %v; want %q", got.Groups.Effect, effect)
	}
}

// TestAgentToJsonOtherVariants verifies non-"owner-group" variants pass
// through without ever producing a Groups object, even if Groups/Effect
// happen to be set (which the schema-level validator should prevent, but
// the conversion itself shouldn't silently wrap them either).
func TestAgentToJsonOtherVariants(t *testing.T) {
	effect := "keep"
	for _, variant := range []string{"any", "mcp-client", "agent-owner", "provider"} {
		t.Run(variant, func(t *testing.T) {
			model := &AgentModel{
				Type:   variant,
				Groups: []GroupModelV1{{Directory: strPtr("okta"), Id: strPtr("1"), Label: strPtr("L")}},
				Effect: &effect,
			}
			got := agentToJson(model)
			if got.Groups != nil {
				t.Errorf("Groups = %+v; want nil for variant %q", got.Groups, variant)
			}
		})
	}
}

// TestAgentToJsonFromJsonRoundTrip verifies agentFromJson(agentToJson(x)) == x
// for the "owner-group" variant (the only one needing conversion) and a
// passthrough variant.
func TestAgentToJsonFromJsonRoundTrip(t *testing.T) {
	effect := "remove"
	cases := []*AgentModel{
		{Type: "owner-group", Groups: []GroupModelV1{{Directory: strPtr("workspace"), Id: strPtr("456"), Label: strPtr("Eng")}}, Effect: &effect},
		{Type: "provider", ProviderId: strPtr("okta-idp"), SubjectPattern: strPtr("^svc-.*$")},
		{Type: "mcp-client", ClientId: strPtr("client-1")},
		{Type: "agent-owner", Owner: strPtr("owner@example.com")},
		{Type: "any"},
	}

	for _, model := range cases {
		t.Run(model.Type, func(t *testing.T) {
			roundTripped := agentFromJson(agentToJson(model))
			if !reflect.DeepEqual(roundTripped, model) {
				t.Errorf("round-trip = %+v; want %+v", roundTripped, model)
			}
		})
	}
}

func TestAgentToJsonFromJsonNil(t *testing.T) {
	if got := agentToJson(nil); got != nil {
		t.Errorf("agentToJson(nil) = %+v; want nil", got)
	}
	if got := agentFromJson(nil); got != nil {
		t.Errorf("agentFromJson(nil) = %+v; want nil", got)
	}
}

// TestUpgradeRequestorV2 verifies the purely-additive V2->V3 requestor
// upgrade: existing fields pass through unchanged, and the new Agent/User
// fields come out nil (no prior state had them).
func TestUpgradeRequestorV2(t *testing.T) {
	effect := "keep"
	uid := "user@example.com"
	prior := &RequestorModelV2{
		Type:   "group",
		Groups: []GroupModelV1{{Directory: strPtr("okta"), Id: strPtr("1"), Label: strPtr("L")}},
		Uid:    &uid,
		Effect: &effect,
	}

	got := upgradeRequestorV2(prior)

	if got.Type != prior.Type || !reflect.DeepEqual(got.Groups, prior.Groups) || got.Uid != prior.Uid || got.Effect != prior.Effect {
		t.Errorf("upgradeRequestorV2 changed a passthrough field: got %+v, prior %+v", got, prior)
	}
	if got.Agent != nil {
		t.Errorf("Agent = %+v; want nil", got.Agent)
	}
	if got.User != nil {
		t.Errorf("User = %+v; want nil", got.User)
	}
}

// TestUpgradeModelV2 verifies the V2->V3 access-policy upgrade wraps the
// requestor upgrade and passes every other field through unchanged.
func TestUpgradeModelV2(t *testing.T) {
	name := "test-policy"
	disabled := true
	prior := AccessPolicyModelV2{
		Name:      &name,
		Disabled:  &disabled,
		Requestor: &RequestorModelV2{Type: "any"},
		Resource:  &ResourceModel{Type: "any"},
		Approval:  []ApprovalModelV2{{Type: "deny"}},
	}

	got := upgradeModelV2(prior)

	if got.Name != prior.Name || got.Disabled != prior.Disabled || got.Resource != prior.Resource {
		t.Errorf("upgradeModelV2 changed a passthrough field: got %+v, prior %+v", got, prior)
	}
	if !reflect.DeepEqual(got.Approval, prior.Approval) {
		t.Errorf("Approval = %+v; want %+v", got.Approval, prior.Approval)
	}
	if got.Requestor.Type != "any" || got.Requestor.Agent != nil || got.Requestor.User != nil {
		t.Errorf("Requestor = %+v; want upgraded passthrough with nil Agent/User", got.Requestor)
	}
}

func strPtr(s string) *string { return &s }

// TestAgenticRequestorWireShape verifies the schema's `agent` and `user` are
// sent as P0's `actor` and `subject`, with the person's rule nested inside the
// subject. P0 rejects a requestor that keeps them as siblings.
func TestAgenticRequestorWireShape(t *testing.T) {
	effect := "keep"
	model := &RequestorModelV5{
		Type:  "agentic",
		Agent: &AgentModel{Type: "any"},
		User: &AgenticUserModelV5{
			Type:   "group",
			Groups: []GroupModelV1{{Directory: strPtr("workspace"), Id: strPtr("123"), Label: strPtr("eng@p0.dev")}},
			Effect: &effect,
		},
	}

	got := requestorToJson(model)

	if got.Actor == nil || got.Actor.Type != "any" {
		t.Fatalf("Actor = %+v; want type %q", got.Actor, "any")
	}
	if got.Subject == nil || got.Subject.Type != "user" {
		t.Fatalf("Subject = %+v; want type %q", got.Subject, "user")
	}
	if got.Subject.User == nil || got.Subject.User.Type != "group" {
		t.Fatalf("Subject.User = %+v; want type %q", got.Subject.User, "group")
	}
	if len(got.Subject.User.Groups) != 1 {
		t.Fatalf("Subject.User.Groups has %d entries; want 1", len(got.Subject.User.Groups))
	}
}

// TestAgenticRequestorHeadless verifies a `user` of "none" becomes an `agent`
// subject, with the actor marked as the same principal.
func TestAgenticRequestorHeadless(t *testing.T) {
	model := &RequestorModelV5{
		Type:  "agentic",
		Agent: &AgentModel{Type: "agent-client", ClientId: strPtr("abc")},
		User:  &AgenticUserModelV5{Type: "none"},
	}

	got := requestorToJson(model)

	if got.Actor == nil || got.Actor.Type != "same-as-subject" {
		t.Fatalf("Actor = %+v; want type %q", got.Actor, "same-as-subject")
	}
	if got.Subject == nil || got.Subject.Type != "agent" {
		t.Fatalf("Subject = %+v; want type %q", got.Subject, "agent")
	}
	if got.Subject.Agent == nil || got.Subject.Agent.Type != "agent-client" {
		t.Fatalf("Subject.Agent = %+v; want type %q", got.Subject.Agent, "agent-client")
	}
}

// TestAgenticRequestorAgentToAgent verifies a `user` of "agent" sends
// `user.agent` as the subject and the requestor `agent` as the actor.
func TestAgenticRequestorAgentToAgent(t *testing.T) {
	model := &RequestorModelV5{
		Type:  "agentic",
		Agent: &AgentModel{Type: "agent-client", ClientId: strPtr("orchestrator")},
		User: &AgenticUserModelV5{
			Type:  "agent",
			Agent: &AgentModel{Type: "provider", ProviderId: strPtr("okta-idp")},
		},
	}

	got := requestorToJson(model)

	if got.Actor == nil || got.Actor.Type != "agent-client" || got.Actor.ClientId == nil || *got.Actor.ClientId != "orchestrator" {
		t.Fatalf("Actor = %+v; want the agent-client orchestrator", got.Actor)
	}
	if got.Subject == nil || got.Subject.Type != "agent" {
		t.Fatalf("Subject = %+v; want type %q", got.Subject, "agent")
	}
	if got.Subject.Agent == nil || got.Subject.Agent.Type != "provider" || got.Subject.Agent.ProviderId == nil || *got.Subject.Agent.ProviderId != "okta-idp" {
		t.Fatalf("Subject.Agent = %+v; want the okta-idp provider", got.Subject.Agent)
	}
	if got.Subject.User != nil {
		t.Errorf("Subject.User = %+v; want nil", got.Subject.User)
	}
}

// TestAgenticRequestorFromJsonAgentSubject verifies how an `agent` subject read
// back from P0 maps onto `user`: a "same-as-subject" (or missing) actor is a
// headless session, and any other actor is agent-to-agent.
func TestAgenticRequestorFromJsonAgentSubject(t *testing.T) {
	subject := &SubjectJson{Type: "agent", Agent: &AgentJson{Type: "agent-owner", Owner: strPtr("owner@p0.dev")}}
	owner := &AgentModel{Type: "agent-owner", Owner: strPtr("owner@p0.dev")}
	cases := []struct {
		name      string
		actor     *AgentJson
		wantAgent *AgentModel
		wantUser  *AgenticUserModelV5
	}{
		{"same-as-subject", &AgentJson{Type: "same-as-subject"}, owner, &AgenticUserModelV5{Type: "none"}},
		{"missing actor", nil, owner, &AgenticUserModelV5{Type: "none"}},
		{"any actor", &AgentJson{Type: "any"}, &AgentModel{Type: "any"}, &AgenticUserModelV5{Type: "agent", Agent: owner}},
		{
			"specific actor",
			&AgentJson{Type: "agent-client", ClientId: strPtr("orchestrator")},
			&AgentModel{Type: "agent-client", ClientId: strPtr("orchestrator")},
			&AgenticUserModelV5{Type: "agent", Agent: owner},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requestorFromJson(RequestorJson{Type: "agentic", Actor: tc.actor, Subject: subject})

			if !reflect.DeepEqual(got.Agent, tc.wantAgent) {
				t.Errorf("Agent = %+v; want %+v", got.Agent, tc.wantAgent)
			}
			if !reflect.DeepEqual(got.User, tc.wantUser) {
				t.Errorf("User = %+v; want %+v", got.User, tc.wantUser)
			}
		})
	}
}

// TestAgenticRequestorRoundTrip verifies each `user` variant survives a
// to-then-from conversion, so a read back from P0 does not show a diff.
func TestAgenticRequestorRoundTrip(t *testing.T) {
	effect := "keep"
	uid := "someone@p0.dev"
	for _, user := range []*AgenticUserModelV5{
		{Type: "any"},
		{Type: "none"},
		{Type: "user", Uid: &uid},
		{
			Type:   "group",
			Groups: []GroupModelV1{{Directory: strPtr("okta"), Id: strPtr("1"), Label: strPtr("Admins")}},
			Effect: &effect,
		},
		{Type: "agent", Agent: &AgentModel{Type: "agent-client", ClientId: strPtr("worker")}},
	} {
		model := &RequestorModelV5{Type: "agentic", Agent: &AgentModel{Type: "any"}, User: user}

		got := requestorFromJson(requestorToJson(model))

		if !reflect.DeepEqual(got.User, user) {
			t.Errorf("User round trip = %+v; want %+v", got.User, user)
		}
		if got.Agent == nil || got.Agent.Type != "any" {
			t.Errorf("Agent round trip = %+v; want type %q", got.Agent, "any")
		}
	}
}

// TestNonAgenticRequestorHasNoActor verifies a group rule is unchanged, and in
// particular carries no agentic fields.
func TestNonAgenticRequestorHasNoActor(t *testing.T) {
	effect := "keep"
	model := &RequestorModelV5{
		Type:   "group",
		Groups: []GroupModelV1{{Directory: strPtr("okta"), Id: strPtr("1"), Label: strPtr("Admins")}},
		Effect: &effect,
	}

	got := requestorToJson(model)

	if got.Actor != nil || got.Subject != nil {
		t.Fatalf("Actor = %+v, Subject = %+v; want both nil", got.Actor, got.Subject)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("Groups has %d entries; want 1", len(got.Groups))
	}
}

// objectWithType builds a non-null object of the given type with every
// attribute null except `type` (a non-pointer string field that can't be null),
// set to typeVal. Lets us populate the agentic agent/user sub-objects without
// hand-writing every field.
func objectWithType(objType tftypes.Object, typeVal string) tftypes.Value {
	attrs := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		if name == "type" {
			attrs[name] = tftypes.NewValue(attrType, typeVal)
			continue
		}
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(objType, attrs)
}

// agenticRequestor builds a non-null `agentic` requestor with non-null
// agent/user sub-objects, forcing the upgrader's decode to descend into the
// requestor and match schema v3's requestor attributes against RequestorModelV3.
func agenticRequestor(t *testing.T, requestorType tftypes.Type) tftypes.Value {
	t.Helper()
	objType, ok := requestorType.(tftypes.Object)
	if !ok {
		t.Fatalf("requestor type is not an object")
	}
	attrs := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		switch name {
		case "type":
			attrs[name] = tftypes.NewValue(attrType, "agentic")
		case "agent", "user":
			subType, ok := attrType.(tftypes.Object)
			if !ok {
				t.Fatalf("%s type is not an object", name)
			}
			attrs[name] = objectWithType(subType, "any")
		default:
			attrs[name] = tftypes.NewValue(attrType, nil)
		}
	}
	return tftypes.NewValue(objType, attrs)
}

// TestUpgradeV3ToV4Approvals exercises the V3->V4 upgrader end to end. It
// deliberately uses a NON-NULL agentic requestor (agent + user populated),
// because that is the only thing that forces the upgrader's decode to descend
// into the requestor object.
func TestUpgradeV3ToV4Approvals(t *testing.T) {
	ctx := context.Background()
	upgrader := (&AccessPolicy{}).UpgradeState(ctx)[3]

	priorType, ok := upgrader.PriorSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("prior schema type is not an object")
	}
	raw := tftypes.NewValue(priorType, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "example"),
		"disabled":  tftypes.NewValue(tftypes.Bool, nil),
		"requestor": agenticRequestor(t, priorType.AttributeTypes["requestor"]),
		"resource":  tftypes.NewValue(priorType.AttributeTypes["resource"], nil),
		"approval":  oneApproval(t, priorType.AttributeTypes["approval"]),
	})

	req := resource.UpgradeStateRequest{State: &tfsdk.State{Schema: *upgrader.PriorSchema, Raw: raw}}
	resp := &resource.UpgradeStateResponse{State: tfsdk.State{Schema: newAccessPolicySchema(currentSchemaVersion)}}
	upgrader.StateUpgrader(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("V3->current upgrade failed: %v", resp.Diagnostics.Errors())
	}
}

// TestUpgradeV4ToV5AgenticUser exercises the V4->V5 upgrader end to end with a
// non-null agentic requestor, so the decode descends into `user`, and checks
// the prior `user` passes through with `user.agent` unset.
func TestUpgradeV4ToV5AgenticUser(t *testing.T) {
	ctx := context.Background()
	upgrader := (&AccessPolicy{}).UpgradeState(ctx)[4]

	priorType, ok := upgrader.PriorSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("prior schema type is not an object")
	}
	raw := tftypes.NewValue(priorType, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "example"),
		"disabled":  tftypes.NewValue(tftypes.Bool, nil),
		"requestor": agenticRequestor(t, priorType.AttributeTypes["requestor"]),
		"resource":  tftypes.NewValue(priorType.AttributeTypes["resource"], nil),
		"approval":  oneApproval(t, priorType.AttributeTypes["approval"]),
	})

	req := resource.UpgradeStateRequest{State: &tfsdk.State{Schema: *upgrader.PriorSchema, Raw: raw}}
	resp := &resource.UpgradeStateResponse{State: tfsdk.State{Schema: newAccessPolicySchema(currentSchemaVersion)}}
	upgrader.StateUpgrader(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("V4->current upgrade failed: %v", resp.Diagnostics.Errors())
	}
	var got AccessPolicyModelV5
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("decoding upgraded state failed: %v", diags.Errors())
	}
	if got.Requestor == nil || got.Requestor.User == nil || got.Requestor.User.Type != "any" || got.Requestor.User.Agent != nil {
		t.Errorf("Requestor.User = %+v; want type %q with no agent", got.Requestor.User, "any")
	}
}

// TestSchemaV5ExposesUserAgent verifies `requestor.user.agent` exists from
// schema v5 onward and not before, so v4 state still decodes.
func TestSchemaV5ExposesUserAgent(t *testing.T) {
	cases := []struct {
		version int64
		want    bool
	}{{4, false}, {5, true}}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("v%d", tc.version), func(t *testing.T) {
			requestor, ok := newAccessPolicySchema(tc.version).Attributes["requestor"].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("requestor attribute is not a SingleNestedAttribute")
			}
			user, ok := requestor.Attributes["user"].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("requestor.user attribute is not a SingleNestedAttribute")
			}
			if _, got := user.Attributes["agent"]; got != tc.want {
				t.Errorf("requestor.user.agent present = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestUpgradeV1ToV2 exercises the registered V1->current state upgrader end to
// end: build to ensure v1 successfully updates the later with the addition of the optional max_duration attribute
func TestUpgradeV1Approval(t *testing.T) {
	ctx := context.Background()
	upgrader := (&AccessPolicy{}).UpgradeState(ctx)[1]

	priorType, ok := upgrader.PriorSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("prior schema type is not an object")
	}
	requestorType, ok := priorType.AttributeTypes["requestor"].(tftypes.Object)
	if !ok {
		t.Fatalf("requestor type is not an object")
	}
	raw := tftypes.NewValue(priorType, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "example"),
		"requestor": objectWithType(requestorType, "any"),
		"resource":  tftypes.NewValue(priorType.AttributeTypes["resource"], nil),
		"approval":  oneApproval(t, priorType.AttributeTypes["approval"]),
	})

	req := resource.UpgradeStateRequest{State: &tfsdk.State{Schema: *upgrader.PriorSchema, Raw: raw}}
	resp := &resource.UpgradeStateResponse{State: tfsdk.State{Schema: newAccessPolicySchema(currentSchemaVersion)}}
	upgrader.StateUpgrader(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("V1 -> current upgrade failed: %v", resp.Diagnostics.Errors())
	}
}

func TestUpgradeV2Approval(t *testing.T) {
	ctx := context.Background()
	upgrader := (&AccessPolicy{}).UpgradeState(ctx)[2]

	priorType, ok := upgrader.PriorSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("prior schema type is not an object")
	}
	requestorType, ok := priorType.AttributeTypes["requestor"].(tftypes.Object)
	if !ok {
		t.Fatalf("requestor type is not an object")
	}
	raw := tftypes.NewValue(priorType, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "example"),
		"disabled":  tftypes.NewValue(tftypes.Bool, nil),
		"requestor": objectWithType(requestorType, "any"),
		"resource":  tftypes.NewValue(priorType.AttributeTypes["resource"], nil),
		"approval":  oneApproval(t, priorType.AttributeTypes["approval"]),
	})

	req := resource.UpgradeStateRequest{State: &tfsdk.State{Schema: *upgrader.PriorSchema, Raw: raw}}
	resp := &resource.UpgradeStateResponse{State: tfsdk.State{Schema: newAccessPolicySchema(currentSchemaVersion)}}
	upgrader.StateUpgrader(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("V2 -> current upgrade failed: %v", resp.Diagnostics.Errors())
	}
}

// builds one non-null approval object of the full element type (every attribute present, values null except the non-pointer type), so the upgrader must decode the nested object and bind each declared attribute to an ApprovalModel field, and ensures they are tested.
func oneApproval(t *testing.T, approvalList tftypes.Type) tftypes.Value {
	t.Helper()
	listType, ok := approvalList.(tftypes.List)
	if !ok {
		t.Fatalf("approval type is not a list")
	}
	elemType, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("approval element type is not an object")
	}
	attrs := map[string]tftypes.Value{}
	for name, attrType := range elemType.AttributeTypes {
		// `type` maps to a non-pointer string field, so it cannot be null.
		if name == "type" {
			attrs[name] = tftypes.NewValue(attrType, "p0")
			continue
		}
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	elem := tftypes.NewValue(elemType, attrs)
	return tftypes.NewValue(approvalList, []tftypes.Value{elem})
}

// TestApprovalMaxDurationRoundTrip verifies a non-null max_duration on an
// approval is persisted under the "maxDuration" JSON key and survives the
// toJson -> toModel conversion.
func TestApprovalMaxDurationRoundTrip(t *testing.T) {
	name := "example"
	maxDuration := duration.Option{Time: 1, Unit: "h"}
	model := AccessPolicyModelV5{
		Name:      &name,
		Requestor: &RequestorModelV5{Type: "any"},
		Resource:  &ResourceModel{Type: "any"},
		Approval:  []ApprovalModelV3{{Type: "p0", MaxDuration: &maxDuration}},
	}

	wire := toJson(model)

	encoded, err := json.Marshal(wire.Approval[0])
	if err != nil {
		t.Fatalf("marshaling approval failed: %v", err)
	}
	if want := `"maxDuration":{"time":1,"unit":"h"}`; !strings.Contains(string(encoded), want) {
		t.Errorf("approval JSON = %s; want it to contain %s", encoded, want)
	}

	got := toModel(wire)

	if len(got.Approval) != 1 {
		t.Fatalf("Approval has %d entries; want 1", len(got.Approval))
	}
	if got.Approval[0].MaxDuration == nil {
		t.Fatalf("MaxDuration is nil; want %+v", maxDuration)
	}
	if *got.Approval[0].MaxDuration != maxDuration {
		t.Errorf("MaxDuration = %+v; want %+v", *got.Approval[0].MaxDuration, maxDuration)
	}
}

// TestSchemaV4ExposesMaxDuration verifies the current schema surfaces the
// approval max_duration attribute (and that it stays gated to version >= 4), so
// the tfsdk-tagged field can actually be read from and written to state.
func TestSchemaV4ExposesMaxDuration(t *testing.T) {
	v4, ok := newAccessPolicySchema(4).Attributes["approval"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("v4 approval attribute is not a ListNestedAttribute")
	}
	if _, ok := v4.NestedObject.Attributes["max_duration"]; !ok {
		t.Errorf("schema v4 approval is missing the max_duration attribute")
	}

	v3, ok := newAccessPolicySchema(3).Attributes["approval"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("v3 approval attribute is not a ListNestedAttribute")
	}
	if _, ok := v3.NestedObject.Attributes["max_duration"]; ok {
		t.Errorf("schema v3 approval unexpectedly exposes max_duration")
	}
}
