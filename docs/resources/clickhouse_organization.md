---
page_title: "p0_clickhouse_organization Resource - p0"
subcategory: ""
description: |-
  A ClickHouse Cloud organization whose services P0 can grant access to, through one of your ClickHouse Cloud connectors.
  The connector acts on the organization with an API key. P0 never holds the key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the organization, and where the API key's secret is.
  Important: Create the API key, store it and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to the organization with the API key, checks that the key's p0-connector role has every permission below except control-plane:service:manage-query-api-endpoints, lists the organization's services, and reads back its own DOMAIN_ALLOW_PATTERN. P0 sets up its read-only Query API endpoints, whose names start with p0_, on a service the first time someone requests SQL or database role access to it or lists its objects. That's when P0 first uses control-plane:service:manage-query-api-endpoints: if the role lacks it, that request or listing fails with a message that names it. If a check fails, or no service in the organization can take requests from P0, the apply fails with P0's message. A service can take requests when it's ready, or when it takes console role requests only (console-only in service_status). A service that isn't ready doesn't fail the apply while another one can take requests: service_status lists it. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.
  P0 fixes the connector, the vault and the secret when it adds the organization, so changing id, connector, vault or api_key_secret_name replaces the installation. If the organization is already added in P0, import it rather than creating it: P0 refuses to add an organization twice. Install a ClickHouse Cloud organization in only one P0 tenant. If P0 hasn't finished installing the organization, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.
  Rotating the API key or adding a service needs nothing in P0 or Terraform. The next SQL or database role request on each service moves P0's Query API endpoints there to the new key, or sets them up on the new service. To rotate the key, create a new key with the same two roles, store it as a new version of the same secret, then delete the old key. api_key_id and service_status keep what the connector read when P0 last validated the install, so they don't show a rotated key or a new service.
  Prerequisites:
  
  A ClickHouse Cloud connector, installed with p0_clickhouse_connector. The API key's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.
  In the ClickHouse Cloud console, a custom role named p0-connector with exactly these permissions, and no others: control-plane:organization:view, control-plane:organization:manage, control-plane:service:view, control-plane:service:view-query-api-endpoints, control-plane:service:manage-query-api-endpoints and control-plane:organization:view-api-keys. control-plane:organization:manage is admin-equivalent: it lets the key create roles and add anyone, itself included, to any role. The connector only ever changes or deletes roles whose names start with p0_.
  An API key with two roles: p0-connector, and the system role Basic service API reader, which running P0's Query API endpoints needs. Store it outside Terraform, as the secret's whole value, as JSON: {"keyId": "...", "keySecret": "..."}, in the account and region or the project that vault names. Storing it outside Terraform keeps it out of Terraform's state.
  Read access to that one secret, and no other, for the connector's identity (its connector_identity). P0 names the secret in each call, so a call could point the connector at any other secret it can read.
  On AWS, give the connector's role secretsmanager:GetSecretValue on arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ?????? matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role kms:Decrypt on that key, with the condition that kms:ViaService is secretsmanager.<secrets_region>.amazonaws.com. A secret outside the connector's account needs a customer-managed key, because no other account can use the default aws/secretsmanager key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.
  On Google Cloud, give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret itself, not on its project.
  P0's secret manager integration for where the secret is: AWS Secrets Manager (p0_aws_secrets_manager) for the AWS account, or Google Secret Manager for the Google Cloud project.
  Note: This integration is currently in beta.
  For the full install steps, see ClickHouse Cloud https://docs.p0.dev/integrations/resource-integrations/clickhouse.
---

# p0_clickhouse_organization (Resource)

A ClickHouse Cloud organization whose services P0 can grant access to, through one of your ClickHouse Cloud connectors.

The connector acts on the organization with an API key. P0 never holds the key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the organization, and where the API key's secret is.

**Important:** Create the API key, store it and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to the organization with the API key, checks that the key's `p0-connector` role has every permission below except `control-plane:service:manage-query-api-endpoints`, lists the organization's services, and reads back its own `DOMAIN_ALLOW_PATTERN`. P0 sets up its read-only Query API endpoints, whose names start with `p0_`, on a service the first time someone requests SQL or database role access to it or lists its objects. That's when P0 first uses `control-plane:service:manage-query-api-endpoints`: if the role lacks it, that request or listing fails with a message that names it. If a check fails, or no service in the organization can take requests from P0, the apply fails with P0's message. A service can take requests when it's ready, or when it takes console role requests only (`console-only` in `service_status`). A service that isn't ready doesn't fail the apply while another one can take requests: `service_status` lists it. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes the connector, the vault and the secret when it adds the organization, so changing `id`, `connector`, `vault` or `api_key_secret_name` replaces the installation. If the organization is already added in P0, import it rather than creating it: P0 refuses to add an organization twice. Install a ClickHouse Cloud organization in only one P0 tenant. If P0 hasn't finished installing the organization, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Rotating the API key or adding a service needs nothing in P0 or Terraform.** The next SQL or database role request on each service moves P0's Query API endpoints there to the new key, or sets them up on the new service. To rotate the key, create a new key with the same two roles, store it as a new version of the same secret, then delete the old key. `api_key_id` and `service_status` keep what the connector read when P0 last validated the install, so they don't show a rotated key or a new service.

**Prerequisites:**

- A ClickHouse Cloud connector, installed with `p0_clickhouse_connector`. The API key's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- In the ClickHouse Cloud console, a custom role named `p0-connector` with exactly these permissions, and no others: `control-plane:organization:view`, `control-plane:organization:manage`, `control-plane:service:view`, `control-plane:service:view-query-api-endpoints`, `control-plane:service:manage-query-api-endpoints` and `control-plane:organization:view-api-keys`. `control-plane:organization:manage` is admin-equivalent: it lets the key create roles and add anyone, itself included, to any role. The connector only ever changes or deletes roles whose names start with `p0_`.

- An API key with two roles: `p0-connector`, and the system role Basic service API reader, which running P0's Query API endpoints needs. Store it outside Terraform, as the secret's whole value, as JSON: `{"keyId": "...", "keySecret": "..."}`, in the account and region or the project that `vault` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its `connector_identity`). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role `secretsmanager:GetSecretValue` on `arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and `??????` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role `kms:Decrypt` on that key, with the condition that `kms:ViaService` is `secretsmanager.<secrets_region>.amazonaws.com`. A secret outside the connector's account needs a customer-managed key, because no other account can use the default `aws/secretsmanager` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (`p0_aws_secrets_manager`) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [ClickHouse Cloud](https://docs.p0.dev/integrations/resource-integrations/clickhouse).

## Example Usage

### AWS

```terraform
# Adds the ClickHouse Cloud organization 4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f,
# whose API key is in AWS Secrets Manager, in the account of the connector on
# AWS Lambda that the p0_clickhouse_connector example installs. For a connector
# on Cloud Run and an API key in Google Secret Manager, see gcp.tf in this
# directory.
#
# Before you apply this, in the ClickHouse Cloud console for the organization,
# create a custom role named p0-connector with exactly these permissions, and
# no others:
#
#   control-plane:organization:view
#   control-plane:organization:manage
#   control-plane:service:view
#   control-plane:service:view-query-api-endpoints
#   control-plane:service:manage-query-api-endpoints
#   control-plane:organization:view-api-keys
#
# control-plane:organization:manage is admin-equivalent: it lets the key create
# roles and add anyone, itself included, to any role. The connector only ever
# changes or deletes roles whose names start with p0_.
#
# Then create an API key with two roles: p0-connector, and the system role
# Basic service API reader, which running P0's Query API endpoints needs. Copy
# its key ID and key secret when the console shows them. The example doesn't
# create the secret, which keeps the API key out of Terraform's state.
#
# Creating p0_clickhouse_organization has the connector read the API key, so
# store it in its secret before then, as JSON: {"keyId": "...", "keySecret":
# "..."}. To store it, with the AWS CLI signed in to the secret's account, read
# the key ID, then the key secret without showing it:
#
#   read -r CLICKHOUSE_KEY_ID && read -rs CLICKHOUSE_KEY_SECRET
#
# Then create the secret:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | aws secretsmanager create-secret \
#     --name clickhouse/acme/api-key \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the API key as a new version instead:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | aws secretsmanager put-secret-value \
#     --secret-id clickhouse/acme/api-key \
#     --secret-string file:///dev/stdin --region us-west-2
#
# Then clear the API key from your shell:
#
#   unset CLICKHOUSE_KEY_ID CLICKHOUSE_KEY_SECRET
#
# To rotate the key, create a new key with the same two roles, store it as a
# new version of the secret, then delete the old key. P0 sets up its Query API
# endpoints on a new service at the first SQL or database role request there.
# Neither needs a change in P0 or in this configuration.
#
# The grant is what P0's ClickHouse Cloud installer generates for the
# organization, with p0_clickhouse_organization after it.

locals {
  p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the organization's API key: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the API
    # key, add a new version to this secret instead.
    api_key_secret_name = "clickhouse/acme/api-key"
    secrets_account_id  = "123456789012"
    secrets_region      = "us-west-2"
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
  alias               = "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f"
  region              = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region
  allowed_account_ids = [local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.connector_account_id]
}

data "aws_partition" "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f" {
  provider = aws.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f
}

# The connector reads the organization's API key, and no other secret. The
# policy goes on the connector's role, which P0 names in connector_identity.
# Each organization adds a policy of its own to that role, and a role's inline
# policies share a limit of 10,240 characters: past a few dozen organizations,
# deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f_secret" {
  provider = aws.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f

  name = "ReadClickHouseApiKey-4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f"
  role = p0_clickhouse_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadApiKey"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region,
          local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id,
          "secret",
          "${local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.api_key_secret_name}-??????",
        ])
      }],
      local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn == null ? [] : [{
        Sid      = "DecryptApiKey"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region}.${data.aws_partition.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id == local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.connector_account_id || local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn != null
      error_message = "The API key's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the API key. IAM changes
# can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_clickhouse_organization" "example" {
  id        = "4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f"
  connector = p0_clickhouse_connector.example.id

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id
    secrets_region = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region
  }
  api_key_secret_name = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.api_key_secret_name

  depends_on = [aws_iam_role_policy.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f_secret]
}

# The email domains the connector acts for, as P0 read them back from it.
output "clickhouse_allowed_domains" {
  value = p0_clickhouse_organization.example.allowed_domains
}

# Each service that isn't ready for every request, with why, as of the
# organization's last validation in the P0 app or by Terraform. Empty when
# every service is ready.
output "clickhouse_services_not_ready" {
  value = p0_clickhouse_organization.example.service_status
}
```

### Google Cloud

```terraform
# The Google Cloud variant of the p0_clickhouse_organization example in this
# directory: the organization's connector runs on Cloud Run (see gcp.tf in the
# p0_clickhouse_connector example), and its API key is in Google Secret
# Manager.
#
# Before you apply this, create the p0-connector role and the API key, as the
# example in resource.tf says, and store the key as the secret
# clickhouse-acme-api-key, as JSON: {"keyId": "...", "keySecret": "..."}. The
# example doesn't create the secret, which keeps the API key out of Terraform's
# state. To store it, read the key ID, then the key secret without showing it:
#
#   read -r CLICKHOUSE_KEY_ID && read -rs CLICKHOUSE_KEY_SECRET
#
# Then create the secret:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | gcloud secrets create clickhouse-acme-api-key \
#     --project my-secrets-project-id --data-file=-
#
# If the secret exists, store the API key as a new version instead:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | gcloud secrets versions add clickhouse-acme-api-key \
#     --project my-secrets-project-id --data-file=-
#
# Then clear the API key from your shell:
#
#   unset CLICKHOUSE_KEY_ID CLICKHOUSE_KEY_SECRET
#
# To rotate the key, store the new one the same way, as resource.tf says.

locals {
  p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d = {
    # The secret that holds the organization's API key, and its project, which
    # needn't be the connector's. A new secret replaces the installation. To
    # rotate the API key, add a new version to this secret instead.
    api_key_secret_project = "my-secrets-project-id"
    api_key_secret_id      = "clickhouse-acme-api-key"
  }
}

# The connector reads the API key's value. Each binding adds one member to the
# secret's IAM policy, and replaces nothing. Both are on the secret and not on
# its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_accessor" {
  project   = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  secret_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_clickhouse_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_viewer" {
  project   = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  secret_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_clickhouse_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the API key.
resource "p0_clickhouse_organization" "gcp_example" {
  id        = "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
  connector = p0_clickhouse_connector.gcp_example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  }
  api_key_secret_name = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_accessor,
    google_secret_manager_secret_iam_member.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `api_key_secret_name` (String) The name of the secret that holds the API key, such as `clickhouse/acme/api-key` in AWS Secrets Manager or `clickhouse-acme-api-key` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. The secret holds the key as JSON: `{"keyId": "...", "keySecret": "..."}`. In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like `-AbCdEf`, because Secrets Manager can't find a secret by such a name. Changing it replaces the installation. To rotate the API key, store the new one as a new version of the same secret, which needs no change here or in P0.
- `connector` (String) The connector that acts on this organization: the `id` of a `p0_clickhouse_connector` resource. Requires that connector to be installed.
- `id` (String) The organization's ID in ClickHouse Cloud, a UUID such as `0b5a3c1e-6f2d-4e8a-9c7b-1d2e3f4a5b6c`, in lowercase. The ClickHouse Cloud console shows it in the organization's details. Add each organization to one P0 tenant only: if two tenants have it, each keeps putting P0's Query API endpoints back to its own API key, and requests in both tenants fail from time to time.
- `vault` (Attributes) The secret manager that holds the API key. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `allowed_domains` (String) The email domains the connector acts for: the `DOMAIN_ALLOW_PATTERN` it's deployed with, as P0 read it back from the connector when it last validated the install.
- `api_key_id` (String) The ID of the API key in the secret, which the connector reads when P0 validates the install. P0 never stores the key's secret.
- `organization_name` (String) The organization's name in ClickHouse Cloud, which the connector reads when P0 validates the install.
- `service_status` (String) Each service that isn't ready for every request, with why, as of the organization's last validation in the P0 app or by Terraform: `<name>: <status> (<reason>)` for each, joined with `; `, where the status is `console-only` or `not-ready`. Empty when every service is ready. It doesn't show services added or changed since then.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: either `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the API key's secret (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the API key's secret (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the API key's secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
