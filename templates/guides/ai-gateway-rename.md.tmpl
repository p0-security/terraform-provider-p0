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

The rename itself changed only names. The gateway's installation in P0 is
untouched, and moving state across the rename destroys nothing.

If you are upgrading from v0.53.0, note that v0.54.0 also changed the gateway
attributes. Update them in the same edit, following the
[`p0_ai_gateway_staged`](https://registry.terraform.io/providers/p0-security/p0/latest/docs/resources/ai_gateway_staged) and
[`p0_ai_gateway`](https://registry.terraform.io/providers/p0-security/p0/latest/docs/resources/ai_gateway) documentation.

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
report each resource as moved, with no changes.

If you are upgrading from v0.53.0, do not plan this migration with
`-refresh=false`. The move carries only what v0.53.0 recorded, so without a
refresh Terraform compares your configuration against empty values and proposes
replacing the staged gateway. State from v0.54.0 is complete, and moves intact
with or without a refresh.

If a refreshing plan still proposes destroying or replacing a gateway, stop:
your configuration differs from what is installed in P0. Correct it before
applying.

Apply in every workspace and state that uses this configuration or module.
Terraform moves a state only when it plans against it, so keep the `moved`
blocks until the change has been applied to every state, then delete them in a
later change.

## Helm chart and Terraform module

The Helm chart and Terraform module were renamed separately. The chart is now
`ai-gateway-stack`, and the module is published as
`p0-security/p0-ai-gateway-stack/kubernetes` from version 0.3.0. Its previous
address, `p0-security/p0-agentic-gateway-stack/kubernetes`, is archived at
0.2.3. Moving to the new address is a `source` and `version` change; the
module's values keys and defaults are unchanged.

Keep your module block's label as it is, even though the examples now call it
`ai_gateway_stack`. Renaming the label moves every resource in the module, so it
needs a `moved` block of its own.
