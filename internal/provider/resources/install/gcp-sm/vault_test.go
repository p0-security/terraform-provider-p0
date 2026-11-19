package installgcpsm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// P0 keys the item by the bare project ID, so the vault named by an integration's
// secret_manager.project_id is the one this resource installs.
func TestGcpSmVaultId(t *testing.T) {
	r := &gcpSmVault{}
	id := r.getId(&gcpSmVaultModel{Project: types.StringValue("my-project")})
	if id == nil || *id != "my-project" {
		t.Fatalf("getId = %v, want my-project", id)
	}
}

func TestGcpSmVaultJson(t *testing.T) {
	r := &gcpSmVault{}

	body, err := json.Marshal(r.toJson(&gcpSmVaultModel{
		Project: types.StringValue("my-project"),
		Label:   types.StringValue("ignored"),
		State:   types.StringValue("ignored"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{}`; string(body) != want {
		t.Errorf("toJson = %s, want %s", body, want)
	}

	var api gcpSmVaultApi
	if err := json.Unmarshal([]byte(`{"item":{"label":"My Project","state":"installed"}}`), &api); err != nil {
		t.Fatal(err)
	}
	var diags diag.Diagnostics
	got, ok := r.fromJson(context.Background(), &diags, "my-project", r.getItemJson(&api)).(*gcpSmVaultModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson failed: %v", diags)
	}
	want := gcpSmVaultModel{
		Project: types.StringValue("my-project"),
		Label:   types.StringValue("My Project"),
		State:   types.StringValue("installed"),
	}
	if *got != want {
		t.Errorf("fromJson = %+v, want %+v", *got, want)
	}
}
