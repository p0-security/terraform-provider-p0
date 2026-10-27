package installlaunchdarkly

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An account that P0 returns without a vault, or with one this provider doesn't
// support, is reported, since a nil vault would otherwise reach Terraform as an
// inconsistent result.
func TestAccountFromJsonVault(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vault   *vaultJson
		summary string
		detail  string
	}{
		{"no vault", nil, "Bad API response", `P0 returned the LaunchDarkly account "prod" without its vault`},
		{"unsupported vault", &vaultJson{Type: "azure-kv", Install: "acme"}, "Unsupported LaunchDarkly account vault", `has a "azure-kv" vault`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked: %v; want a diagnostic", r)
				}
			}()
			var diags diag.Diagnostics
			model := accountFromJson(context.Background(), &diags, "prod", &accountJson{Vault: tc.vault})
			errs := diags.Errors()
			if model != nil || len(errs) != 1 || errs[0].Summary() != tc.summary || !strings.Contains(errs[0].Detail(), tc.detail) {
				t.Errorf("model = %v, diagnostics = %v; want no model and one %q with %q", model, diags, tc.summary, tc.detail)
			}
		})
	}
}

// The instance is a select, which P0 stores by its option's type, and
// read back the same way. A select P0 doesn't have reads as null.
func TestAccountSelects(t *testing.T) {
	model := &accountModel{
		Connector:       types.StringValue("prod"),
		Instance:        types.StringValue("eu"),
		TokenSecretName: types.StringValue("launchdarkly-prod-token"),
	}
	json, ok := accountToJson(model).(*accountJson)
	if !ok || json.Instance == nil || json.Instance.Type != "eu" {
		t.Fatalf("accountToJson = %#v; want the instance by its type", json)
	}
	if json.AccountId != nil || json.TokenKind != nil || json.TokenOwnerMemberId != nil || json.AllowedDomains != nil {
		t.Errorf("accountToJson = %#v; want none of the fields P0 reads from the connector", json)
	}

	if got := selectValue(&selectJson{Type: "federal"}); got != types.StringValue("federal") {
		t.Errorf("selectValue of federal = %v", got)
	}
	if got := selectValue(nil); !got.IsNull() {
		t.Errorf("selectValue of no select = %v; want null", got)
	}

	unknown, ok := accountToJson(&accountModel{Instance: types.StringUnknown()}).(*accountJson)
	if !ok || unknown.Instance != nil {
		t.Errorf("accountToJson of an unknown instance = %#v; want none sent", unknown)
	}
}

// A refusal to change a fixed field isn't a failed check of the customer's setup, so
// it has a summary of its own. Every refusal says how to start over.
func TestDescribeAccountCheckError(t *testing.T) {
	for _, tc := range []struct {
		name, message, summary string
	}{
		{
			"a failed check",
			"422 Unprocessable Entity: The LaunchDarkly account prod failed its install check while reading the access token's secret: the secret doesn't exist.",
			"P0 didn't finish installing the LaunchDarkly account",
		},
		{
			"a changed field",
			"422 Unprocessable Entity: 'instance' can only be altered on initial installation. Create a new installation to change this field.",
			"LaunchDarkly account differs from the one in P0",
		},
	} {
		summary, detail := describeAccountCheckError("prod", errors.New(tc.message))
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

// A vault with no type gives no JSON.
func TestVaultToJson(t *testing.T) {
	if json := (&vaultModel{}).toJson(); json != nil {
		t.Errorf("a vault with no type gives %v; want nil", json)
	}
}

// An answer from P0 with no item reads as none, so the install flow reports it rather
// than hand FromJson a nil item.
func TestAccountItemJsonWithoutItem(t *testing.T) {
	if item := accountItemJson(&accountApi{}); item != nil {
		t.Errorf("accountItemJson of an answer with no item = %#v; want nil", item)
	}
	if item := connectorItemJson(&connectorApi{}); item != nil {
		t.Errorf("connectorItemJson of an answer with no item = %#v; want nil", item)
	}
}
