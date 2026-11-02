// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// RequirePreviewResources adds an error to diags unless the provider block sets
// enable_preview_resources = true. Preview resources call it from ModifyPlan and
// ImportState; ValidateConfig runs before the provider is configured, so the flag
// isn't known there.
//
// A nil data means the provider isn't configured yet, so there is nothing to check.
func RequirePreviewResources(data *P0ProviderData, typeName string, diags *diag.Diagnostics) {
	if data == nil || data.PreviewResourcesEnabled {
		return
	}
	diags.AddError(
		"Preview resource not enabled",
		fmt.Sprintf("%s is a preview resource; set enable_preview_resources = true in the p0 provider block to use it.", typeName),
	)
}
