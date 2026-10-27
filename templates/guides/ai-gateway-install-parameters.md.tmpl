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
| `storage_class`         | `postgresql`, `victorialogs` and `valkey` `.storageClass` in the module's `values` |
| `kubernetes_namespace`  | the module's `namespace` input                              |

P0 did not use these values, so deleting them from the configuration changes
nothing about the gateway's installation in P0.

## New required attribute

`p0_ai_gateway` now requires `braekhus_public_jwk`: the gateway's public tunnel
key, an EC P-384 public JWK. P0 uses it to reach a gateway that has no public
address. A configuration upgrading to this version must set it, so apply the
module first and read the key from the running gateway:

```shell
kubectl -n <namespace> exec deploy/braekhus-client -- cat /p0-files/jwk.public.json
```

## Module and chart

Use module version 1.3.0 or later with chart version 1.3.0-beta or later. The
module requires `chart_version`, and the chart's values key is `ai-gateway`
(previously `agentic-gateway`), with the component keys `authServer` and
`accessProxy`. See the module's README for its own upgrade notes.

```terraform
module "ai_gateway_stack" {
  source        = "p0-security/p0-ai-gateway-stack/kubernetes"
  version       = "1.3.0"
  chart_version = "1.3.0-beta"
  # ...
}
```
