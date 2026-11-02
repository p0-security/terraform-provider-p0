// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func TestRequirePreviewResourcesRefusesWhenDisabled(t *testing.T) {
	var diags diag.Diagnostics
	RequirePreviewResources(&P0ProviderData{}, "p0_access_bundle", &diags)

	if diags.ErrorsCount() != 1 || diags.WarningsCount() != 0 {
		t.Fatalf("want exactly one error and no warnings, got %v", diags)
	}
	want := "p0_access_bundle is a preview resource; set enable_preview_resources = true in the p0 provider block to use it."
	if got := diags.Errors()[0].Detail(); !strings.Contains(got, want) {
		t.Errorf("error detail = %q, want it to contain %q", got, want)
	}
}

func TestRequirePreviewResourcesAllowsWhenEnabled(t *testing.T) {
	var diags diag.Diagnostics
	RequirePreviewResources(&P0ProviderData{PreviewResourcesEnabled: true}, "p0_access_bundle", &diags)

	if len(diags) != 0 {
		t.Errorf("want no diagnostics, got %v", diags)
	}
}

func TestRequirePreviewResourcesSkipsUnconfiguredProvider(t *testing.T) {
	var diags diag.Diagnostics
	RequirePreviewResources(nil, "p0_access_bundle", &diags)

	if len(diags) != 0 {
		t.Errorf("want no diagnostics, got %v", diags)
	}
}
