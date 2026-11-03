// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"encoding/json"
	"testing"
)

func mustRule(t *testing.T, raw string) AccessPolicyJson {
	t.Helper()
	var rule AccessPolicyJson
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		t.Fatal(err)
	}
	return rule
}

// TestBundlePolicyMatches checks the comparison of a bundle with its
// `bundle:<id>` rule: P0's defaults apply when the bundle leaves requestor and
// approval unset, an unset disabled equals false, and the nulls the bundle
// keeps but the rule drops don't count.
func TestBundlePolicyMatches(t *testing.T) {
	keep := "keep"
	disabled := true
	notDisabled := false
	requireReason := true
	groupApproval := []ApprovalModelV3{{
		Type:    "group",
		Effect:  &keep,
		Groups:  []GroupModelV1{{Directory: strPtr("workspace"), Id: strPtr("123"), Label: strPtr("Approvers")}},
		Options: &ApprovalOptionsModel{RequireReason: &requireReason},
	}}
	groupApprovalRule := `[{"type":"group","effect":"keep","groups":[{"directory":"workspace","id":"123","label":"Approvers"}],"options":{"requireReason":true}}]`
	emptyApproval := []ApprovalModelV3{}

	tests := []struct {
		name   string
		bundle AccessBundleJson
		rule   string
		want   bool
	}{
		{
			name: "defaults",
			rule: `{"requestor":{"type":"any"},"approval":[{"type":"p0"}]}`,
			want: true,
		},
		{
			name:   "empty approval defaults to a security reviewer",
			bundle: AccessBundleJson{Approval: &emptyApproval},
			rule:   `{"requestor":{"type":"any"},"approval":[{"type":"p0"}]}`,
			want:   true,
		},
		{
			name:   "set approval",
			bundle: AccessBundleJson{Approval: &groupApproval},
			rule:   `{"requestor":{"type":"any"},"approval":` + groupApprovalRule + `}`,
			want:   true,
		},
		{
			name:   "approval changed",
			bundle: AccessBundleJson{Approval: &groupApproval},
			rule:   `{"requestor":{"type":"any"},"approval":[{"type":"p0"}]}`,
			want:   false,
		},
		{
			name: "requestor changed",
			rule: `{"requestor":{"type":"user","uid":"someone@example.com"},"approval":[{"type":"p0"}]}`,
			want: false,
		},
		{
			name:   "unset disabled equals false",
			bundle: AccessBundleJson{Disabled: &notDisabled},
			rule:   `{"requestor":{"type":"any"},"approval":[{"type":"p0"}]}`,
			want:   true,
		},
		{
			name:   "disabled changed",
			bundle: AccessBundleJson{Disabled: &disabled},
			rule:   `{"requestor":{"type":"any"},"approval":[{"type":"p0"}]}`,
			want:   false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bundlePolicyMatches(test.bundle, mustRule(t, test.rule)); got != test.want {
				t.Errorf("bundlePolicyMatches() = %t, want %t", got, test.want)
			}
		})
	}
}
