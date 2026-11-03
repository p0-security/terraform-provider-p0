package installmiroai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A team that P0 returns without a vault, or with one this provider doesn't support,
// is reported, since a nil vault would otherwise reach Terraform as an inconsistent
// result.
func TestTeamFromJsonVault(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vault   *vaultJson
		summary string
		detail  string
	}{
		{"no vault", nil, "Bad API response", `P0 returned the Miro team "design" without its vault`},
		{"unsupported vault", &vaultJson{Type: "azure-kv", Install: "acme"}, "Unsupported Miro team vault", `has a "azure-kv" vault`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked: %v; want a diagnostic", r)
				}
			}()
			var diags diag.Diagnostics
			model := teamFromJson(context.Background(), &diags, "design", &teamJson{Vault: tc.vault})
			errs := diags.Errors()
			if model != nil || len(errs) != 1 || errs[0].Summary() != tc.summary || !strings.Contains(errs[0].Detail(), tc.detail) {
				t.Errorf("model = %v, diagnostics = %v; want no model and one %q with %q", model, diags, tc.summary, tc.detail)
			}
		})
	}
}

// The request carries every field the form asks for, and none that P0 reads from the
// connector.
func TestTeamToJson(t *testing.T) {
	model := &teamModel{
		Connector:       types.StringValue("prod"),
		TokenSecretName: types.StringValue("miro-design-token"),
		TeamId:          types.StringValue("3458764688635236553"),
		ShareableBoards: types.StringValue("2"),
	}
	json, ok := teamToJson(model).(*teamJson)
	if !ok || json.Connector == nil || *json.Connector != "prod" || json.TokenSecretName == nil || *json.TokenSecretName != "miro-design-token" {
		t.Fatalf("teamToJson = %#v; want the connector and the token's secret", json)
	}
	if json.TeamId != nil || json.ShareableBoards != nil || json.State != nil {
		t.Errorf("teamToJson = %#v; want none of the fields P0 sets", json)
	}
}

// What P0 reads from the connector comes back on the model as it stored it.
func TestTeamFromJsonReported(t *testing.T) {
	teamId, boards := "3458764688635236553", "0"
	var diags diag.Diagnostics
	model, ok := teamFromJson(context.Background(), &diags, "design", &teamJson{
		Vault:           &vaultJson{Type: "gcp-sm", Install: "acme-secrets"},
		TeamId:          &teamId,
		ShareableBoards: &boards,
	}).(*teamModel)
	if !ok || diags.HasError() {
		t.Fatalf("teamFromJson = %#v, diagnostics = %v; want a model", model, diags)
	}
	if model.TeamId.ValueString() != teamId || model.ShareableBoards.ValueString() != boards {
		t.Errorf("teamFromJson = %#v; want team_id %q and shareable_boards %q", model, teamId, boards)
	}
}

// A refusal to change a fixed field isn't a failed check of the customer's setup, so
// it has a summary of its own. Every refusal says how to start over.
func TestDescribeTeamCheckError(t *testing.T) {
	for _, tc := range []struct {
		name, message, summary string
	}{
		{
			"a failed check",
			"422 Unprocessable Entity: The Miro team design failed its install check while checking the token's scopes: Add the scope to the app, then reinstall it and store the new token.",
			"P0 didn't finish installing the Miro team",
		},
		{
			"a changed field",
			"422 Unprocessable Entity: 'tokenSecretName' can only be altered on initial installation. Create a new installation to change this field.",
			"Miro team differs from the one in P0",
		},
	} {
		summary, detail := describeTeamCheckError("design", errors.New(tc.message))
		if summary != tc.summary {
			t.Errorf("%s: summary = %q; want %q", tc.name, summary, tc.summary)
		}
		for _, part := range []string{`"design"`, tc.message, "terraform apply -replace=<address>", "terraform state rm <address>"} {
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
func TestTeamItemJsonWithoutItem(t *testing.T) {
	if item := teamItemJson(&teamApi{}); item != nil {
		t.Errorf("teamItemJson of an answer with no item = %#v; want nil", item)
	}
	if item := connectorItemJson(&connectorApi{}); item != nil {
		t.Errorf("connectorItemJson of an answer with no item = %#v; want nil", item)
	}
}
