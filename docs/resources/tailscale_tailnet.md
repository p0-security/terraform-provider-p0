---
page_title: "p0_tailscale_tailnet Resource - p0"
subcategory: ""
description: |-
  A tailnet that P0 can grant network access in, through one of your Tailscale connectors. Requestors pick a P0 posture from the ones your policy file defines, listed live through the connector, and your access policies decide who can request what.
  P0 grants access by setting a posture attribute, with an expiry, on the requestor's device, and never writes your policy file, so you can manage the policy file as code, for example with tailscale_acl. Your policy file gates grants on P0 postures with srcPosture. Posture attributes need a Tailscale Premium or Enterprise plan.
  The connector acts on the tailnet with a Tailscale credential that P0 never holds:
  federated (the default in P0's installer): a Tailscale federated identity that trusts the connector's own cloud identity, so no secret is stored anywhere. Create it with tailscale_federated_identity, with the connector's cloud as its issuer and the connector's identity as its subject: on AWS, the issuer of AWS outbound identity federation for the connector's account and the connector's role ARN; on Google Cloud, https://accounts.google.com and the connector's service account's unique numeric ID, not its email. Leave its audience unset: P0 uses Tailscale's default, api.tailscale.com/<client ID>. On AWS, the connector's role also needs sts:GetWebIdentityToken for Tailscale's audiences, api.tailscale.com/*, for 300 seconds or less, which the connector's Terraform grants.oauth-client: a Tailscale OAuth client whose secret is in your secret manager, in the connector's cloud. Create the client in Tailscale's admin console and store its secret with your cloud's CLI, as the example does, so the secret never enters Terraform's state. The connector reads the secret itself, so let its identity read that one secret and no other.
  The credential needs these scopes: policy_file:read, users:read, devices:core:read, devices:posture_attributes, devices:routes:read and services:read. P0 refuses a credential that also carries policy_file or all, since it could write the policy file.
  Important: Create the credential, and let the connector use it, before Terraform creates this resource: in an earlier apply, or in the same apply with this resource depending on the grants, as the examples do. Creating this resource has P0 validate the install through the connector: the connector gets a token with the credential, checks its scopes, checks that it can set a posture attribute, which needs a Tailscale Premium or Enterprise plan, and counts the P0 postures the tailnet's policy file defines. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied right after you grant it, apply again.
  P0 fixes a tailnet's connector and credential when it adds the tailnet, and its open grants were made through them. Replacing the tailnet would remove it from P0 without revoking those grants, so Terraform refuses a change to either at plan time. To change one, revoke the tailnet's grants, then remove this resource, apply, and add it again. A new id is another tailnet, and replaces this one. Destroying this resource removes the tailnet from P0 and revokes nothing, so revoke its grants first. If the tailnet is already added in P0, import it rather than creating it: P0 refuses to add a tailnet twice. If P0 hasn't finished installing the tailnet, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.
  Prerequisites:
  
  A Tailscale Premium or Enterprise plan for the tailnet.
  A Tailscale connector, installed with p0_tailscale_connector. An OAuth client's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.
  For a federated identity on AWS, AWS outbound identity federation turned on for the connector's account.
  For an OAuth client, P0's secret manager integration for where the secret is: AWS Secrets Manager (p0_aws_secrets_manager) for the AWS account, or Google Secret Manager for the Google Cloud project.
  Note: This integration is currently in beta.
  For the full install steps, see Tailscale https://docs.p0.dev/integrations/resource-integrations/tailscale.
---

# p0_tailscale_tailnet (Resource)

A tailnet that P0 can grant network access in, through one of your Tailscale connectors. Requestors pick a P0 posture from the ones your policy file defines, listed live through the connector, and your access policies decide who can request what.

P0 grants access by setting a posture attribute, with an expiry, on the requestor's device, and never writes your policy file, so you can manage the policy file as code, for example with `tailscale_acl`. Your policy file gates grants on P0 postures with `srcPosture`. Posture attributes need a Tailscale Premium or Enterprise plan.

The connector acts on the tailnet with a Tailscale credential that P0 never holds:

- `federated` (the default in P0's installer): a Tailscale federated identity that trusts the connector's own cloud identity, so no secret is stored anywhere. Create it with `tailscale_federated_identity`, with the connector's cloud as its issuer and the connector's identity as its subject: on AWS, the issuer of AWS outbound identity federation for the connector's account and the connector's role ARN; on Google Cloud, `https://accounts.google.com` and the connector's service account's unique numeric ID, not its email. Leave its `audience` unset: P0 uses Tailscale's default, `api.tailscale.com/<client ID>`. On AWS, the connector's role also needs `sts:GetWebIdentityToken` for Tailscale's audiences, `api.tailscale.com/*`, for 300 seconds or less, which the connector's Terraform grants.
- `oauth-client`: a Tailscale OAuth client whose secret is in your secret manager, in the connector's cloud. Create the client in Tailscale's admin console and store its secret with your cloud's CLI, as the example does, so the secret never enters Terraform's state. The connector reads the secret itself, so let its identity read that one secret and no other.

The credential needs these scopes: `policy_file:read`, `users:read`, `devices:core:read`, `devices:posture_attributes`, `devices:routes:read` and `services:read`. P0 refuses a credential that also carries `policy_file` or `all`, since it could write the policy file.

**Important:** Create the credential, and let the connector use it, before Terraform creates this resource: in an earlier apply, or in the same apply with this resource depending on the grants, as the examples do. Creating this resource has P0 validate the install through the connector: the connector gets a token with the credential, checks its scopes, checks that it can set a posture attribute, which needs a Tailscale Premium or Enterprise plan, and counts the P0 postures the tailnet's policy file defines. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied right after you grant it, apply again.

P0 fixes a tailnet's `connector` and `credential` when it adds the tailnet, and its open grants were made through them. Replacing the tailnet would remove it from P0 without revoking those grants, so Terraform refuses a change to either at plan time. To change one, revoke the tailnet's grants, then remove this resource, apply, and add it again. A new `id` is another tailnet, and replaces this one. Destroying this resource removes the tailnet from P0 and revokes nothing, so revoke its grants first. If the tailnet is already added in P0, import it rather than creating it: P0 refuses to add a tailnet twice. If P0 hasn't finished installing the tailnet, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- A Tailscale Premium or Enterprise plan for the tailnet.

- A Tailscale connector, installed with `p0_tailscale_connector`. An OAuth client's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- For a federated identity on AWS, AWS outbound identity federation turned on for the connector's account.

- For an OAuth client, P0's secret manager integration for where the secret is: AWS Secrets Manager (`p0_aws_secrets_manager`) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [Tailscale](https://docs.p0.dev/integrations/resource-integrations/tailscale).

## Example Usage

### Federated identity, connector on AWS Lambda

```terraform
# Adds the tailnet T1234CNTRL through the connector on AWS Lambda that the
# p0_tailscale_connector example installs. The tailnet needs a Tailscale Premium
# or Enterprise plan, and a policy file that gates grants on P0 postures, as the
# Google Cloud example's does. The connector signs in with a Tailscale federated
# identity that trusts the connector's role, so no secret is stored anywhere. For
# a connector on Cloud Run, see the Google Cloud example (gcp.tf). For an OAuth
# client whose secret is in your secret manager, see the OAuth client example
# (oauth-client.tf).
#
# Tailscale's own Terraform provider creates the federated identity, signed in to
# the same tailnet with a credential of yours that may create trust credentials.
# The connector's own Terraform turns on AWS outbound identity federation and
# lets the connector's role mint the identity tokens.
#
# This example goes in the same configuration as the p0_tailscale_connector
# example, and uses its required_providers block. If your configuration already
# has a required_providers block, add these to it:
#   p0 = {
#     source = "p0-security/p0"
#   }
#   tailscale = {
#     source = "tailscale/tailscale"
#   }

locals {
  p0_tailscale_tailnet_t1234cntrl = {
    # The Tailnet ID from Tailscale's Settings, General, not the tailnet's DNS
    # name.
    tailnet_id = "T1234CNTRL"
    # The scopes the connector needs, which never let it write the policy file.
    # A federated identity's tokens carry all of them.
    scopes = [
      "policy_file:read",
      "users:read",
      "devices:core:read",
      "devices:posture_attributes",
      "devices:routes:read",
      "services:read",
    ]
  }
}

# Set TAILSCALE_OAUTH_CLIENT_ID and TAILSCALE_OAUTH_CLIENT_SECRET, or the
# provider's other credential settings, for your own credential.
provider "tailscale" {
  tailnet = local.p0_tailscale_tailnet_t1234cntrl.tailnet_id
}

# Trusts the AWS identity tokens that the connector's role mints. Leave audience
# unset: P0 uses Tailscale's default, api.tailscale.com/<client ID>.
resource "tailscale_federated_identity" "p0" {
  description = "P0 connector prod"
  issuer      = aws_iam_outbound_web_identity_federation.p0_tailscale_connector_prod.issuer_identifier
  subject     = p0_tailscale_connector.example.connector_identity.role_arn
  scopes      = local.p0_tailscale_tailnet_t1234cntrl.scopes
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector, so it waits until the connector's role may mint the token. IAM
# changes can take a few seconds to propagate, so if the connector is denied,
# apply again.
resource "p0_tailscale_tailnet" "example" {
  id        = local.p0_tailscale_tailnet_t1234cntrl.tailnet_id
  connector = p0_tailscale_connector.example.id

  credential = {
    type      = "federated"
    client_id = tailscale_federated_identity.p0.id
  }

  depends_on = [aws_iam_role_policy.p0_tailscale_connector_prod_identity_token]
}

# What P0 found on the tailnet when it validated it.
output "tailscale_discovery" {
  value = p0_tailscale_tailnet.example.discovery
}
```

### Federated identity, connector on Google Cloud Run

```terraform
# The Google Cloud variant of the p0_tailscale_tailnet example: the tailnet's
# connector runs on Cloud Run (see the p0_tailscale_connector example's Google
# Cloud Run variant, gcp.tf), and tailscale_acl owns the tailnet's policy file,
# which gates grants on P0 postures.
#
# The federated identity trusts the Google ID tokens of the connector's service
# account. A Google ID token's subject is the service account's unique numeric
# ID, not its email, so the data source looks the ID up.

locals {
  p0_tailscale_tailnet_t5678cntrl = {
    tailnet_id = "T5678CNTRL"
    # The scopes the connector needs, which never let it write the policy file.
    scopes = [
      "policy_file:read",
      "users:read",
      "devices:core:read",
      "devices:posture_attributes",
      "devices:routes:read",
      "services:read",
    ]
  }
}

provider "tailscale" {
  alias   = "p0_tailscale_tailnet_t5678cntrl"
  tailnet = local.p0_tailscale_tailnet_t5678cntrl.tailnet_id
}

data "google_service_account" "p0_tailscale_connector_gcp" {
  account_id = p0_tailscale_connector.gcp_example.connector_identity.service_account_email
}

# Leave audience unset: P0 uses Tailscale's default, api.tailscale.com/<client ID>.
resource "tailscale_federated_identity" "p0_gcp" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  description = "P0 connector prod-gcp"
  issuer      = "https://accounts.google.com"
  subject     = data.google_service_account.p0_tailscale_connector_gcp.unique_id
  scopes      = local.p0_tailscale_tailnet_t5678cntrl.scopes
}

# tailscale_acl owns the policy file, so this turns on "prevent edits in the
# admin console" to keep edits made there from being lost on the next apply.
resource "tailscale_tailnet_settings" "p0_gcp" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  acls_externally_managed_on = true
}

# The policy file, which tailscale_acl replaces whole on every apply, so keep
# all of the tailnet's rules here. It defines a P0 posture that requestors can
# ask for, and gates a grant on it with srcPosture: a device gets the grant
# while P0 sets custom:p0_prod_db on it.
resource "tailscale_acl" "gitops" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  overwrite_existing_content = true
  acl = jsonencode({
    tagOwners = {
      "tag:prod-db" = ["autogroup:admin"]
    }
    postures = {
      "posture:p0_prod_db" = ["custom:p0_prod_db == true"]
    }
    grants = [{
      src        = ["autogroup:member"]
      dst        = ["tag:prod-db"]
      ip         = ["tcp:5432"]
      srcPosture = ["posture:p0_prod_db"]
    }]
  })
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector: the connector exchanges its own ID token for a Tailscale token.
resource "p0_tailscale_tailnet" "gcp_example" {
  id        = local.p0_tailscale_tailnet_t5678cntrl.tailnet_id
  connector = p0_tailscale_connector.gcp_example.id

  credential = {
    type      = "federated"
    client_id = tailscale_federated_identity.p0_gcp.id
  }

  # P0 counts the P0 postures the policy file defines when it validates the
  # tailnet.
  depends_on = [tailscale_acl.gitops]
}
```

### OAuth client

```terraform
# The OAuth client variant of the p0_tailscale_tailnet example, for a tailnet
# where a federated identity can't be used: the connector reads a Tailscale OAuth
# client's secret from AWS Secrets Manager, in the account of the connector on
# AWS Lambda that the p0_tailscale_connector example installs.
#
# Before you apply this, create the OAuth client in Tailscale's admin console,
# under Settings > Trust credentials, with the scopes the p0_tailscale_tailnet
# resource lists, and store its secret as the secret tailscale/acme/oauth. The
# example doesn't create the client or the secret, which keeps the client's
# secret out of Terraform's state.
#
# Creating p0_tailscale_tailnet has the connector read the secret, so store it
# before then, as the secret's whole value. To store it, with the AWS CLI signed
# in to the secret's account, read the client's secret without showing it:
#
#   read -rs TAILSCALE_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$TAILSCALE_CLIENT_SECRET" | aws secretsmanager create-secret \
#     --name tailscale/acme/oauth \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the client's secret as a new version instead,
# which is also how to rotate it:
#
#   printf %s "$TAILSCALE_CLIENT_SECRET" | aws secretsmanager put-secret-value \
#     --secret-id tailscale/acme/oauth \
#     --secret-string file:///dev/stdin --region us-west-2

locals {
  p0_tailscale_tailnet_t9012cntrl = {
    tailnet_id = "T9012CNTRL"
    # The OAuth client's ID, from Tailscale's Trust credentials page.
    client_id = "kAbc1CNTRL"
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the OAuth client's secret: its name, and the account
    # and region it's in. P0 has the connector read the secret by this name
    # there. To rotate the client's secret, add a new version to this secret.
    client_secret_name = "tailscale/acme/oauth"
    secrets_account_id = "123456789012"
    secrets_region     = "us-west-2"
    # The ARN of the KMS key that encrypts the secret, if that's a
    # customer-managed key. The default aws/secretsmanager key needs nothing
    # for a secret in the connector's account, but can't be used from another
    # one. For a secret in another account, set this to its customer-managed
    # key, and in that account allow the connector's role
    # secretsmanager:GetSecretValue in the secret's resource policy and
    # kms:Decrypt through Secrets Manager in the key's policy.
    kms_key_arn = null
  }
}

# IAM is global, so the region only has to be one you use. Add the credential
# settings your other AWS providers use, such as profile or assume_role, if
# they have any.
provider "aws" {
  alias               = "p0_tailscale_tailnet_t9012cntrl"
  region              = local.p0_tailscale_tailnet_t9012cntrl.secrets_region
  allowed_account_ids = [local.p0_tailscale_tailnet_t9012cntrl.connector_account_id]
}

data "aws_partition" "p0_tailscale_tailnet_t9012cntrl" {
  provider = aws.p0_tailscale_tailnet_t9012cntrl
}

# The connector reads this tailnet's OAuth client secret, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity.
resource "aws_iam_role_policy" "p0_tailscale_tailnet_t9012cntrl_secret" {
  provider = aws.p0_tailscale_tailnet_t9012cntrl

  name = "ReadTailscaleClientSecret-${local.p0_tailscale_tailnet_t9012cntrl.tailnet_id}"
  role = p0_tailscale_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadClientSecret"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_tailscale_tailnet_t9012cntrl.secrets_region,
          local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id,
          "secret",
          "${local.p0_tailscale_tailnet_t9012cntrl.client_secret_name}-??????",
        ])
      }],
      local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn == null ? [] : [{
        Sid      = "DecryptClientSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_tailscale_tailnet_t9012cntrl.secrets_region}.${data.aws_partition.p0_tailscale_tailnet_t9012cntrl.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id == local.p0_tailscale_tailnet_t9012cntrl.connector_account_id || local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn != null
      error_message = "The OAuth client's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the secret. IAM changes
# can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_tailscale_tailnet" "oauth_example" {
  id        = local.p0_tailscale_tailnet_t9012cntrl.tailnet_id
  connector = p0_tailscale_connector.example.id

  credential = {
    type      = "oauth-client"
    client_id = local.p0_tailscale_tailnet_t9012cntrl.client_id
    vault = {
      type           = "aws-sm"
      account_id     = local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id
      secrets_region = local.p0_tailscale_tailnet_t9012cntrl.secrets_region
    }
    client_secret_name = local.p0_tailscale_tailnet_t9012cntrl.client_secret_name
  }

  depends_on = [aws_iam_role_policy.p0_tailscale_tailnet_t9012cntrl_secret]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The connector that acts on this tailnet: the `id` of a `p0_tailscale_connector` resource. Requires that connector to be installed. It can't change.
- `credential` (Attributes) The Tailscale credential the connector uses for this tailnet. P0 never holds a secret for it. It can't change. (see [below for nested schema](#nestedatt--credential))
- `id` (String) The Tailnet ID from Tailscale's Settings, General, such as `T1234CNTRL`: the `tailnet` your `tailscale` provider is configured with. A tailnet created before October 2025 also answers to its legacy ID, such as `example.com`. The tailnet's DNS name, such as `tail1234.ts.net`, can't be used: Tailscale's API doesn't take it. Changing it replaces the installation, which removes the tailnet from P0 and revokes nothing, so revoke the tailnet's grants first.

### Read-Only

- `discovery` (String) What P0 found on the tailnet when it last validated it, such as its P0 postures.
- `dns_name` (String) The tailnet's DNS name, such as `tail1234.ts.net`, as P0 found it when it last validated the tailnet. Requestors and approvers see it next to the Tailnet ID. Empty when the tailnet has no device of its own.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--credential"></a>
### Nested Schema for `credential`

Required:

- `client_id` (String) The client ID of the federated identity or OAuth client, as Tailscale's Trust credentials page shows it, such as `kAbc1CNTRL`. For a federated identity, the `id` of its `tailscale_federated_identity`.
- `type` (String) How the connector authenticates to Tailscale: either `federated` (a federated identity that trusts the connector's own cloud identity) or `oauth-client` (an OAuth client whose secret is in your secret manager)

Optional:

- `client_secret_name` (String) The name of the secret that holds, or will hold, the OAuth client's secret, such as `tailscale/oauth-client` in AWS Secrets Manager or `tailscale-oauth-client` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. Required for, and only valid with, `oauth-client`. In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like `-AbCdEf`, because Secrets Manager can't find a secret by such a name. To rotate the secret, store the new one as a new version of the same secret, which needs no change here.
- `vault` (Attributes) The secret manager that holds the OAuth client's secret. Required for, and only valid with, `oauth-client`. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there. (see [below for nested schema](#nestedatt--credential--vault))

<a id="nestedatt--credential--vault"></a>
### Nested Schema for `credential.vault`

Required:

- `type` (String) The secret manager: either `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the OAuth client's secret (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the OAuth client's secret (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the OAuth client's secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
