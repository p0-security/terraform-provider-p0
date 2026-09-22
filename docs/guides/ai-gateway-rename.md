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

Note that if you are upgrading from v0.53.0 you also cross an unrelated schema
change released alongside this rename, which replaced the gateway's `url` and
`oauth_endpoint` attributes with a nested `domain_hosting` block. Update those
attributes in the same edit as the type names. Terraform will plan a change for
them, and on `p0_ai_gateway_staged` it may plan a replacement, because
`domain_hosting` cannot be altered after installation. That change comes from the
new attribute, not from the rename — a plan reporting it has still moved your
state correctly.

## Terraform 1.8 and later

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

Run `terraform plan`. It should report the moves, plus any `domain_hosting`
change noted above and nothing else. Apply, then delete the `moved` blocks in a
later change.

Resources declared with `count` or `for_each` move per instance, so include the
index or key: `from = p0_agentic_server.example["aws-tools"]`.

## Terraform 1.7 and earlier

`moved` blocks cannot change a resource type before Terraform 1.8. Rename the
types in your configuration, then move each state entry from the command line:

```shell
terraform state mv p0_agentic_gateway_staged.example p0_ai_gateway_staged.example
terraform state mv p0_agentic_gateway.example p0_ai_gateway.example
terraform state mv p0_agentic_server.aws_example p0_ai_gateway_server.aws_example
```

Then run `terraform plan` and confirm it reports only the changes noted above.

## What did not change

The Helm chart `agentic-gateway-stack` and the Terraform module
`p0-security/p0-agentic-gateway-stack/kubernetes` keep their names, so module
blocks referring to them need no edit.
