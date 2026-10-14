package installharbor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// An instance that P0 returns without a vault, or with one this provider doesn't
// support, is reported, since a nil vault would otherwise reach Terraform as an
// inconsistent result.
func TestInstanceFromJsonVault(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vault   *vaultJson
		summary string
		detail  string
	}{
		{"no vault", nil, "Bad API response", `P0 returned the Harbor instance "prod" without its vault`},
		{"unsupported vault", &vaultJson{Type: "azure-kv", Install: "acme"}, "Unsupported Harbor instance vault", `has a "azure-kv" vault`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked: %v; want a diagnostic", r)
				}
			}()
			var diags diag.Diagnostics
			model := instanceFromJson(context.Background(), &diags, "prod", &instanceJson{Vault: tc.vault})
			errs := diags.Errors()
			if model != nil || len(errs) != 1 || errs[0].Summary() != tc.summary || !strings.Contains(errs[0].Detail(), tc.detail) {
				t.Errorf("model = %v, diagnostics = %v; want no model and one %q with %q", model, diags, tc.summary, tc.detail)
			}
		})
	}
}

// A refusal to change a fixed field isn't a failed check of the customer's setup, so
// it has a summary of its own. Every refusal says how to start over.
func TestDescribeInstanceCheckError(t *testing.T) {
	for _, tc := range []struct {
		name, message, summary string
	}{
		{"a failed check", "422 Unprocessable Entity: The connector couldn't read the secret.", "P0 didn't finish installing the Harbor instance"},
		{
			"a changed field",
			"422 Unprocessable Entity: 'url' can only be altered on initial installation. Create a new installation to change this field.",
			"Harbor instance differs from the one in P0",
		},
	} {
		summary, detail := describeInstanceCheckError("prod", errors.New(tc.message))
		if summary != tc.summary {
			t.Errorf("%s: summary = %q; want %q", tc.name, summary, tc.summary)
		}
		for _, part := range []string{`"prod"`, tc.message, "terraform apply -replace=<address>", "terraform state rm <address>"} {
			if !strings.Contains(detail, part) {
				t.Errorf("%s: detail = %q; want it to hold %q", tc.name, detail, part)
			}
		}
	}
}

// The vault's JSON for each type, and none for a type this provider doesn't support.
func TestVaultToJson(t *testing.T) {
	if json := (&vaultModel{}).toJson(); json != nil {
		t.Errorf("a vault with no type gives %v; want nil", json)
	}
}

// An answer from P0 with no item reads as none, so the install flow reports it rather
// than hand FromJson a nil item.
func TestInstanceItemJsonWithoutItem(t *testing.T) {
	if item := instanceItemJson(&instanceApi{}); item != nil {
		t.Errorf("instanceItemJson of an answer with no item = %#v; want nil", item)
	}
}
