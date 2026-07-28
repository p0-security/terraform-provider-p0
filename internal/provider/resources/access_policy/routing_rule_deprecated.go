// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &RoutingRule{}
var _ resource.ResourceWithImportState = &RoutingRule{}
var _ resource.ResourceWithUpgradeState = &RoutingRule{}

// RoutingRule is the deprecated predecessor of AccessPolicy. It manages the
// same P0 API object under the old p0_routing_rule resource type so that
// existing configurations keep working while they migrate.
type RoutingRule struct {
	AccessPolicy
}

func NewRoutingRule() resource.Resource {
	return &RoutingRule{}
}

func (rule *RoutingRule) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rule"
}

func (rule *RoutingRule) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	deprecated := newAccessPolicySchema(currentSchemaVersion)
	deprecated.DeprecationMessage = "Use the p0_access_policy resource instead. p0_routing_rule will be removed in a future release."
	deprecated.MarkdownDescription = `~> **Deprecated** Routing rules are now called access policies. Use the ` + "`p0_access_policy`" + ` resource instead;
this resource will be removed in a future release.

To migrate without recreating anything, rename the resource type in your configuration and add a ` + "`moved`" + ` block
(requires Terraform 1.8 or later):

` + "```terraform" + `
moved {
  from = p0_routing_rule.example
  to   = p0_access_policy.example
}
` + "```" + `

An access policy that controls who can request access to what, and access requirements.
See [the P0 access-policy docs](https://docs.p0.dev/just-in-time-access/request-routing).`
	resp.Schema = deprecated
}
