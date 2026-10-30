---
page_title: "Upgrading: AI gateway installation parameters"
subcategory: ""
description: |-
  The p0_ai_gateway_staged resource no longer accepts the Helm chart's installation parameters, and p0_ai_gateway requires the gateway's tunnel public key.
---

# Upgrading: AI gateway installation parameters

P0 no longer stores the Helm chart's installation parameters. They are chart
values, and they now live only in the `values` of the
[`p0-security/p0-ai-gateway-stack/kubernetes`](https://registry.terraform.io/modules/p0-security/p0-ai-gateway-stack/kubernetes/latest)
module.

## Removed attributes

The following attributes were removed from `p0_ai_gateway_staged` rather than
deprecated, so a configuration that sets them will not plan until they are
deleted:

| Removed attribute       | Where the value goes now                                    |
| ----------------------- | ----------------------------------------------------------- |
| `lets_encrypt_email`    | `letsEncrypt.email` in the module's `values`                |
| `oidc_client_id`        | `ai-gateway.authServer.oidcClientId` in the module's `values` |
| `storage_class`         | `postgresql` and `victorialogs` `.storageClass` in the module's `values` |
| `kubernetes_namespace`  | the module's `namespace` input                              |

P0 did not use these values, so deleting them from the configuration changes
nothing about the gateway's installation in P0.

Do not copy the old `storage_class` into `valkey.storageClass`. Valkey holds
credentials, so it needs an encrypted storage class, which the old value may
not be. Choose one for Valkey separately.

## New required attribute

`p0_ai_gateway` now requires `braekhus_public_jwk`: the gateway's public tunnel
key, an EC P-384 public JWK. P0 uses it to connect to the gateway over the
braekhus tunnel, so a gateway with no public address can be reached. The key
exists only after the chart is installed, so the install takes two applies, the
same as `p0_kubernetes` and its `public_jwk`.

The second apply succeeds only if P0 can connect to the running gateway with
that key. A successful apply therefore confirms the tunnel works. P0 itself
still accepts a gateway without a key, to support older gateway versions that
have no tunnel. Those fall back to a direct call to the gateway's address. This
provider does not offer that path.

### Fresh install

1. Define only `p0_ai_gateway_staged` and the module, with no `p0_ai_gateway`
   block, then run `terraform apply`. Terraform checks every resource block in
   the configuration for required arguments, even when you use `-target`, so a
   `p0_ai_gateway` block without `braekhus_public_jwk` stops the plan.

2. Read the key from the running braekhus client pod. The braekhus client
   generates the file at startup, so the command works with or without the
   `ai-gateway-stack` chart:

   ```shell
   kubectl exec -it deploy/braekhus-client -n <namespace> -c braekhus -- cat /p0-files/jwk.public.json | jq -Mr
   ```

3. Add the `p0_ai_gateway` block with `braekhus_public_jwk` set to that key,
   then run `terraform apply` again.

### Upgrading an existing install

Gateways installed from older chart versions have no braekhus client, so there
is no key to read yet. Upgrade the cluster first, while the provider version
you already use is still in place. That version does not require the key, so
`p0_ai_gateway` keeps applying.

1. Upgrade the module to 1.4.0 and `chart_version` to 1.4.0-beta, as described
   under [Module and chart](#module-and-chart), and run `terraform apply`.

2. Read the key from the braekhus client pod with the command above.

3. Upgrade the provider, delete the removed attributes from
   `p0_ai_gateway_staged`, set `braekhus_public_jwk` on the existing
   `p0_ai_gateway`, and run `terraform apply` again.

## Module and chart

Use module version 1.4.0 or later with chart version 1.4.0-beta or later. The
module requires `chart_version`, and the chart's values key is `ai-gateway`
(previously `agentic-gateway`), with the component keys `authServer` and
`accessProxy`. See the module's README for its own upgrade notes.

```terraform
module "ai_gateway_stack" {
  source        = "p0-security/p0-ai-gateway-stack/kubernetes"
  version       = "1.4.0"
  chart_version = "1.4.0-beta"
  # ...
}
```
