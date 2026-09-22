---
page_title: "Upgrading: AI gateway resource rename"
subcategory: ""
description: |-
  The agentic gateway resource types were renamed to match the AI gateway product name. Existing configurations need a type rename and a state move.
---

# Upgrading: AI gateway resource rename

The Agentic gateway is now the AI gateway, and its resource types were renamed to
match:

| Previous name               | Current name            |
| --------------------------- | ----------------------- |
| `p0_agentic_gateway`        | `p0_ai_gateway`         |
| `p0_agentic_gateway_staged` | `p0_ai_gateway_staged`  |
| `p0_agentic_server`         | `p0_ai_gateway_server`  |

The previous names were removed rather than deprecated, so a configuration using
them will not plan until it is updated.

The rename changed names only: the schemas are untouched by it, and so is the
gateway's installation in P0. Moving state across the rename destroys nothing.

If you are upgrading from v0.53.0, you also cross an unrelated schema change
released alongside this rename. The gateway's `url` and `oauth_endpoint` moved
into a nested `domain_hosting` block, and the staged gateway gained
`lets_encrypt_email`, `oidc_client_id` and `storage_class`. Update those
attributes in the same edit as the type names, with the values your gateway is
installed with. The move carries the staged gateway's `url` across, and a
refreshing plan reads the rest from P0.

## Migrating

This migration requires Terraform 1.8 or later, the first version whose `moved`
blocks can change a resource type. Earlier versions have no equivalent:
`terraform state mv` refuses to change a resource type.

Rename the type on each resource and each reference to it, then add a `moved`
block per resource. Terraform matches the pair and moves the state entry.

```terraform
resource "p0_ai_gateway_staged" "example" {
  id = "primary"
  # ...unchanged...
}

resource "p0_ai_gateway" "example" {
  id = p0_ai_gateway_staged.example.id
  # ...unchanged...
}

moved {
  from = p0_agentic_gateway_staged.example
  to   = p0_ai_gateway_staged.example
}

moved {
  from = p0_agentic_gateway.example
  to   = p0_ai_gateway.example
}
```

A `moved` block names the whole resource, so one block also moves every
instance of a resource declared with `count` or `for_each`. If the resources are
declared inside a module, put the `moved` blocks in that module.

Run `terraform plan` with refresh enabled, which is the default. It should
report each resource as moved, with no changes. Do not plan this migration with
`-refresh=false`: the move carries only what v0.53.0 recorded, so without a
refresh Terraform compares your configuration against empty values and proposes
replacing the staged gateway. If a refreshing plan still proposes destroying or
replacing a gateway, stop — your configuration differs from what is installed in
P0.

Apply in every workspace and state that uses this configuration or module.
Terraform only moves a state when it plans against it, so keep the `moved`
blocks until every one of them has been applied, and delete them in a later
change.

## What did not change

The Helm chart `agentic-gateway-stack` and the Terraform module
`p0-security/p0-agentic-gateway-stack/kubernetes` keep their names, so module
blocks referring to them need no edit.
