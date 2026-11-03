---
page_title: "p0_access_bundle Resource - p0"
subcategory: ""
description: |-
  An access bundle: a set of access items that users request together, in one request.
  Each bundle owns a policy rule named bundle:<id>, which P0 writes from the bundle's requestor, approval and disabled settings.
  This resource is in preview: set enable_preview_resources = true in the provider block to use it.
---

# p0_access_bundle (Resource)

~> **Preview.** Access bundles are generally available in P0, but this resource is in preview. Set `enable_preview_resources = true` in the provider block to use it. Its schema may change in a minor version of this provider, so pin the provider version.

An access bundle: a set of access items that users request together, in one request.

Each bundle owns a policy rule named `bundle:<id>`, which P0 writes from the bundle's `requestor`, `approval` and `disabled` settings.

This resource is in preview: set `enable_preview_resources = true` in the provider block to use it.

## Example Usage

```terraform
# Access bundles need enable_preview_resources in the provider block:
#
# provider "p0" {
#   org                      = "my-org"
#   enable_preview_resources = true
# }
#
# The gcloud and aws integrations must be installed in your P0 app, and the
# group must exist in an installed directory integration.
resource "p0_access_bundle" "deploy" {
  label       = "Terraform deploy"
  description = "Everything needed to run the production Terraform deploy"

  requestor = {
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "workspace"
      id        = "04d34og81j3pxk1"
      label     = "eng@example.com"
    }]
  }

  approval = [{
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "workspace"
      id        = "00xvir7l2dtpb3m"
      label     = "approvers@example.com"
    }]
    options = {
      require_reason = true
    }
  }]

  items = [
    # A project-level role.
    {
      type   = "gcloud"
      access = "role"
      args = jsonencode({
        project = "my-project"
        name    = ["roles/editor"]
      })
    },
    # A role on a single bucket. The locator is itself a JSON string, so it is
    # encoded once inside args, and args is encoded again.
    {
      type   = "gcloud"
      access = "resource"
      args = jsonencode({
        project  = "my-project"
        accesses = ["roles/storage.objectViewer"]
        locator = jsonencode({
          fullName = "//storage.googleapis.com/my-bucket"
          name     = "my-bucket"
          service  = "storage"
          type     = "bucket"
        })
      })
    },
    # An AWS IAM Identity Center permission set.
    {
      type   = "aws"
      access = "permission-set"
      args = jsonencode({
        account = "123456789012"
        name    = "arn:aws:sso:::permissionSet/ssoins-1234567890abcdef/ps-1234567890abcdef"
      })
    },
  ]
}
```

Each item's `args` is the item's arguments as a JSON object, the same arguments a request for that access takes. Write it with `jsonencode`. Key order and whitespace in `args` don't cause a diff.

## Import

Find the bundle's ID in the P0 app: on the **Policies** page, switch to **Access Bundles** and open the bundle. The ID is the last part of the page's URL, after `bundles/edit/`. Then write an `import` block and let Terraform generate the bundle's configuration:

```terraform
# Import an existing bundle by its ID, then let Terraform write its config:
#
#   terraform plan -generate-config-out=bundles.tf
import {
  to = p0_access_bundle.deploy
  id = "dsy9eYJIUoyBM1RJuvUx"
}
```

Run `terraform plan -generate-config-out=bundles.tf`, review `bundles.tf`, and apply. A plan after the apply shows no changes.

## The bundle's policy rule

P0 makes a bundle requestable through a policy rule named `bundle:<id>`, which this resource reports as `policy_name`. P0 writes the rule from the bundle's `requestor`, `approval` and `disabled`. When `requestor` is unset, anyone can request the bundle, and when `approval` is unset or empty, a P0 security reviewer must approve the request.

Manage the rule through this resource, not by editing it. If the rule is deleted or changed outside Terraform, for example in the policy editor, the next plan shows an update to this bundle with a warning, and applying it rewrites the rule.

`p0_access_policy` can't manage a rule whose name starts with `bundle:`, because that rule belongs to a bundle: P0 would overwrite the policy's changes the next time the bundle changes, and destroying the policy would delete the rule. If you imported a bundle's rule as `p0_access_policy` before this resource existed, move it over:

1. Replace the `p0_access_policy` resource with a `removed` block, so Terraform forgets it without deleting the rule.
2. Import the bundle as `p0_access_bundle`, using the ID after `bundle:`.

```terraform
# A bundle's policy rule was imported as p0_access_policy before
# p0_access_bundle existed. Remove the rule from state without deleting it...
removed {
  from = p0_access_policy.deploy_bundle

  lifecycle {
    destroy = false
  }
}

# ...and import the bundle that owns it, by the ID after "bundle:".
import {
  to = p0_access_bundle.deploy
  id = "dsy9eYJIUoyBM1RJuvUx"
}
```

## Editing a bundle in both Terraform and the P0 app

Terraform owns the whole bundle. If someone edits it in the P0 app, the next plan shows the difference, and applying it puts back the Terraform configuration. To keep a change made in the app, copy it into the configuration before the next apply.

If the bundle is deleted in the P0 app, the next plan creates it again, with a new ID.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `items` (Attributes List) The access the bundle grants, from 2 to 15 items. P0 keeps them in the order given. (see [below for nested schema](#nestedatt--items))
- `label` (String) The bundle's name, as requestors see it.

### Optional

- `approval` (Attributes List) How requests for the bundle are approved. If unset or empty, a P0 security reviewer must approve them. See [the Approval docs](https://docs.p0.dev/just-in-time-access/request-routing#approval). (see [below for nested schema](#nestedatt--approval))
- `description` (String) A description of the bundle, as requestors see it.
- `disabled` (Boolean) Whether the bundle can't be requested. Defaults to false.
- `requestor` (Attributes) Who may request the bundle. If unset, anyone may request it. See [the Requestor docs](https://docs.p0.dev/just-in-time-access/request-routing#requestor). (see [below for nested schema](#nestedatt--requestor))

### Read-Only

- `created_by` (String) Who created the bundle: a user's email address, or an API key's name.
- `id` (String) The bundle's ID, assigned by P0.
- `policy_name` (String) The name of the policy rule P0 keeps for this bundle, `bundle:<id>`.
- `updated_at` (Number) When the bundle last changed, in milliseconds since the Unix epoch.

<a id="nestedatt--items"></a>
### Nested Schema for `items`

Required:

- `access` (String) The kind of access within the integration, for example `role` or `permission-set`.
- `args` (String) The item's arguments, as a JSON object. Use `jsonencode`. Key order and whitespace don't matter.
- `type` (String) The integration that grants this item, for example `gcloud` or `aws`.


<a id="nestedatt--approval"></a>
### Nested Schema for `approval`

Required:

- `type` (String) Determines trust requirements for access. If empty, access is disallowed. Except for 'deny', meeting any requirement is sufficient to grant access. Possible values:
    - 'auto': Access is granted according to the requirements of the specified 'integration'
    - 'deny': Access is always denied
    - 'escalation': Access may be approved by on-call members of the specified services, who are paged when access is manually escalated by the requestor
    - 'group': Access may be granted by any member of the defined directory group
    - 'persistent': Access is always granted
    - 'requestor-profile': Allows approval by a user specified by a field in the requestor's IDP profile
    - 'p0': Access may be granted by any user with the P0 "security reviewer" role (defined in the P0 app)

Optional:

- `directory` (String) Required, and may only be used, if 'type' is 'requestor-profile'. One of "azure-ad", "entra-id", "okta", or "workspace".
- `effect` (String) Required, and may only be used, if 'type' is 'group'. The filter effect. May be one of:
	 - 'keep': Access rule only applies when a requestor is a member of any of the specified groups
	 - 'remove': Access rule only applies when a requestor is _not_ a member of any of the specified groups
- `groups` (Attributes List) Required, and may only be used, if 'type' is 'group'. If the user is a member of any of these groups, the rule will match. (see [below for nested schema](#nestedatt--approval--groups))
- `integration` (String) Required, and may only be used, if 'type' is 'auto' or 'escalation'. Possible values:
- 'pagerduty': Access is granted if the requestor is on-call in PagerDuty.
- 'incidentio': Access is granted if the requestor is on-call in incident.io.
- 'grafana-irm': Access is granted if the requestor is on-call for a Grafana IRM team.
- `max_duration` (Attributes) Allows policy routing based on how long of a duration was requested as part of an access request. max_duration is the maximum request duration that will match this particular policy approval. When not set, requested duration will not affect policy routing and will follow default behavior. (see [below for nested schema](#nestedatt--approval--max_duration))
- `options` (Attributes) If present, determines additional trust requirements. (see [below for nested schema](#nestedatt--approval--options))
- `profile_property` (String) May only be used if 'type' is 'requestor-profile'. This is the profile attribute that contains the manager's email.
- `services` (List of String) Required, and may only be used, if 'type' is 'escalation'. Defines which services to page on escalation.

<a id="nestedatt--approval--groups"></a>
### Nested Schema for `approval.groups`

Required:

- `directory` (String) One of "azure-ad", "entra-id", "okta", or "workspace".
- `id` (String) This is the directory's internal group identifier.
- `label` (String) This is any human-readable name for the directory group specified in the 'id' attribute.


<a id="nestedatt--approval--max_duration"></a>
### Nested Schema for `approval.max_duration`

Required:

- `time` (Number) The number of `unit`s in this duration. Must be a positive integer.
- `unit` (String) The duration unit. One of `s` (seconds), `m` (minutes), `h` (hours), `d` (days), or `w` (weeks).


<a id="nestedatt--approval--options"></a>
### Nested Schema for `approval.options`

Optional:

- `allow_one_party` (Boolean) If true, allows requestors to approve their own requests. Does not apply to 'auto' approval rules.
- `break_glass_approver` (Boolean) If true, allows the approver to approve break-glass requests. Does not apply to 'auto' approval rules.
- `require_duration` (Boolean) If true, requires access requests to include a duration.
- `require_preapproval` (Boolean) If true, requires access requests to be pre-approved.
- `require_reason` (Boolean) If true, requires access requests to include a reason.



<a id="nestedatt--requestor"></a>
### Nested Schema for `requestor`

Required:

- `type` (String) How P0 matches requestors:
    - 'any': Any requestor will match
    - 'group': Members of a directory group will match
    - 'user': Only match a single user
    - 'agentic': Match agent sessions, based on the agent's identity and the human user (if any) behind it

Optional:

- `agent` (Attributes) Required, and may only be used, if the requestor 'type' is 'agentic'. Describes the agent's own identity. (see [below for nested schema](#nestedatt--requestor--agent))
- `effect` (String) Required, and may only be used, if 'type' is 'group'. The filter effect. May be one of:
	 - 'keep': Access rule only applies when a requestor is a member of any of the specified groups
	 - 'remove': Access rule only applies when a requestor is _not_ a member of any of the specified groups
- `groups` (Attributes List) Required, and may only be used, if 'type' is 'group'. If the user is a member of any of these groups, the rule will match. (see [below for nested schema](#nestedatt--requestor--groups))
- `uid` (String) Required, and may only be used, if 'type' is 'user'. This is the user's email address.
- `user` (Attributes) Required, and may only be used, if the requestor 'type' is 'agentic'. Describes the human user (if any) behind the agent, or the agent it acts on behalf of. (see [below for nested schema](#nestedatt--requestor--user))

<a id="nestedatt--requestor--agent"></a>
### Nested Schema for `requestor.agent`

Required:

- `type` (String) How P0 matches the agent:
    - 'any': Any agent will match
    - 'agent-client': Only agents connecting through a specific gateway client will match ('mcp-client' is a deprecated alias)
    - 'agent-owner': Only an agent owned by a specific user will match
    - 'owner-group': Only an agent owned by a member of a directory group will match
    - 'provider': Only an agent federated by a specific identity provider will match

Optional:

- `client_id` (String) Required, and may only be used, if 'type' is 'agent-client' (or its deprecated alias 'mcp-client'). The gateway client's identifier.
- `effect` (String) Required, and may only be used, if 'type' is 'owner-group'. The filter effect. May be one of:
    - 'keep': Access rule only applies when the agent's owner is a member of any of the specified groups
    - 'remove': Access rule only applies when the agent's owner is _not_ a member of any of the specified groups
- `groups` (Attributes List) Required, and may only be used, if 'type' is 'owner-group'. If the agent's owner is a member of any of these groups, the rule will match. (see [below for nested schema](#nestedatt--requestor--agent--groups))
- `owner` (String) Required, and may only be used, if 'type' is 'agent-owner'. The agent owner's email address.
- `provider_id` (String) Required, and may only be used, if 'type' is 'provider'. The identifier of an installed identity-provider integration.
- `subject_pattern` (String) May only be used if 'type' is 'provider'. An optional regular expression used to further narrow the agent's subject claim.

<a id="nestedatt--requestor--agent--groups"></a>
### Nested Schema for `requestor.agent.groups`

Required:

- `directory` (String) One of "azure-ad", "entra-id", "okta", or "workspace".
- `id` (String) This is the directory's internal group identifier.
- `label` (String) This is any human-readable name for the directory group specified in the 'id' attribute.



<a id="nestedatt--requestor--groups"></a>
### Nested Schema for `requestor.groups`

Required:

- `directory` (String) One of "azure-ad", "entra-id", "okta", or "workspace".
- `id` (String) This is the directory's internal group identifier.
- `label` (String) This is any human-readable name for the directory group specified in the 'id' attribute.


<a id="nestedatt--requestor--user"></a>
### Nested Schema for `requestor.user`

Required:

- `type` (String) How P0 matches who the agent acts for:
    - 'any': Any user, or no user, will match
    - 'group': Members of a directory group will match
    - 'user': Only match a single user
    - 'none': Only match a headless agent session with no human user
    - 'agent': Match a session whose subject is an agent matching 'agent' below. The requestor 'agent' is the agent making the request; the two may be the same agent

Optional:

- `agent` (Attributes) Required, and may only be used, if 'type' is 'agent'. Describes the agent that the acting agent works on behalf of. (see [below for nested schema](#nestedatt--requestor--user--agent))
- `effect` (String) Required, and may only be used, if 'type' is 'group'. The filter effect. May be one of:
    - 'keep': Access rule only applies when the human user behind the agent is a member of any of the specified groups
    - 'remove': Access rule only applies when the human user behind the agent is _not_ a member of any of the specified groups
- `groups` (Attributes List) Required, and may only be used, if 'type' is 'group'. If the human user behind the agent is a member of any of these groups, the rule will match. (see [below for nested schema](#nestedatt--requestor--user--groups))
- `uid` (String) Required, and may only be used, if 'type' is 'user'. This is the user's email address.

<a id="nestedatt--requestor--user--agent"></a>
### Nested Schema for `requestor.user.agent`

Required:

- `type` (String) How P0 matches the agent:
    - 'any': Any agent will match
    - 'agent-client': Only agents connecting through a specific gateway client will match ('mcp-client' is a deprecated alias)
    - 'agent-owner': Only an agent owned by a specific user will match
    - 'owner-group': Only an agent owned by a member of a directory group will match
    - 'provider': Only an agent federated by a specific identity provider will match

Optional:

- `client_id` (String) Required, and may only be used, if 'type' is 'agent-client' (or its deprecated alias 'mcp-client'). The gateway client's identifier.
- `effect` (String) Required, and may only be used, if 'type' is 'owner-group'. The filter effect. May be one of:
    - 'keep': Access rule only applies when the agent's owner is a member of any of the specified groups
    - 'remove': Access rule only applies when the agent's owner is _not_ a member of any of the specified groups
- `groups` (Attributes List) Required, and may only be used, if 'type' is 'owner-group'. If the agent's owner is a member of any of these groups, the rule will match. (see [below for nested schema](#nestedatt--requestor--user--agent--groups))
- `owner` (String) Required, and may only be used, if 'type' is 'agent-owner'. The agent owner's email address.
- `provider_id` (String) Required, and may only be used, if 'type' is 'provider'. The identifier of an installed identity-provider integration.
- `subject_pattern` (String) May only be used if 'type' is 'provider'. An optional regular expression used to further narrow the agent's subject claim.

<a id="nestedatt--requestor--user--agent--groups"></a>
### Nested Schema for `requestor.user.agent.groups`

Required:

- `directory` (String) One of "azure-ad", "entra-id", "okta", or "workspace".
- `id` (String) This is the directory's internal group identifier.
- `label` (String) This is any human-readable name for the directory group specified in the 'id' attribute.



<a id="nestedatt--requestor--user--groups"></a>
### Nested Schema for `requestor.user.groups`

Required:

- `directory` (String) One of "azure-ad", "entra-id", "okta", or "workspace".
- `id` (String) This is the directory's internal group identifier.
- `label` (String) This is any human-readable name for the directory group specified in the 'id' attribute.
