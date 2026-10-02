package installawssm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// P0 keys the item by the bare account ID, so the vault named by p0_github_repositories'
// vault.account_id is the one this resource installs.
func TestAwsSmVaultId(t *testing.T) {
	r := &awsSmVault{}
	id := r.getId(&awsSmVaultModel{AccountId: types.StringValue("123456789012")})
	if id == nil || *id != "123456789012" {
		t.Fatalf("getId = %v, want 123456789012", id)
	}
}

func TestAwsSmVaultJson(t *testing.T) {
	r := &awsSmVault{}

	body, err := json.Marshal(r.toJson(&awsSmVaultModel{
		AccountId:     types.StringValue("123456789012"),
		DefaultRegion: types.StringValue("us-west-2"),
		Label:         types.StringValue("ignored"),
		State:         types.StringValue("ignored"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"defaultRegion":"us-west-2"}`; string(body) != want {
		t.Errorf("toJson = %s, want %s", body, want)
	}

	var api awsSmVaultApi
	if err := json.Unmarshal([]byte(`{"item":{"defaultRegion":"us-east-1","label":"prod","state":"installed"}}`), &api); err != nil {
		t.Fatal(err)
	}
	var diags diag.Diagnostics
	got, ok := r.fromJson(context.Background(), &diags, "123456789012", r.getItemJson(&api)).(*awsSmVaultModel)
	if !ok || diags.HasError() {
		t.Fatalf("fromJson failed: %v", diags)
	}
	want := awsSmVaultModel{
		AccountId:     types.StringValue("123456789012"),
		DefaultRegion: types.StringValue("us-east-1"),
		Label:         types.StringValue("prod"),
		State:         types.StringValue("installed"),
	}
	if *got != want {
		t.Errorf("fromJson = %+v, want %+v", *got, want)
	}
}

func TestAwsRegionRegex(t *testing.T) {
	for _, region := range []string{"us-west-2", "eu-central-1", "ap-southeast-3", "us-gov-west-1"} {
		if !AwsRegionRegex.MatchString(region) {
			t.Errorf("%s should be a region", region)
		}
	}
	for _, region := range []string{"", "us-west", "US-WEST-2", "us-west-2a", "arn:aws:secretsmanager:us-west-2"} {
		if AwsRegionRegex.MatchString(region) {
			t.Errorf("%q should not be a region", region)
		}
	}
}
