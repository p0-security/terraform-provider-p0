---
page_title: "p0_launchdarkly_account Resource - p0"
subcategory: ""
description: |-
  A LaunchDarkly account whose existing members P0 can grant base roles, teams, custom roles and role attributes to, through one of your LaunchDarkly connectors. Requests name the account by its id, as in acme-prod/writer.
  P0 never holds the account's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector reaches the account, where the account is hosted, its plan, and where the token's secret is.
  Important: Install the connector first, store the token, and grant the connector's identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the account through the connector: the connector reads the secret, signs in to LaunchDarkly with the token, checks the account's plan, and reads its own allowed email domains. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.
  P0 can hold each LaunchDarkly account only once. It refuses an account whose token is for a LaunchDarkly account it already has.
  An account's attributes can't change once it is created. Changing id, connector, instance, plan, vault or token_secret_name replaces the account. To rotate the token, add a new version to the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the account from P0. P0 can't revoke access it granted on an account it no longer has, so that access stays in LaunchDarkly until you remove it there. Before you change an attribute or destroy this resource, revoke the account's open access in P0 or wait for it to expire.
  An account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: terraform apply -replace=<address> deletes the account and creates it again, and terraform state rm <address> stops managing it.
  The email domains that may receive access are set on the connector, as its DOMAIN_ALLOW_PATTERN environment variable, and aren't an attribute of this resource. P0 reads them back as allowed_domains.
  Prerequisites:
  
  A LaunchDarkly connector, installed with p0_launchdarkly_connector.
  An access token for the account. On Enterprise, a service token with an inline policy that allows what P0 grants. On any other plan, the personal access token of a LaunchDarkly admin with the Admin role: P0 then grants base roles only, and the token stops working when that admin leaves LaunchDarkly.
  A secret that holds the token alone, as plain text, in the secret manager that vault names. Store it outside Terraform, which keeps the token out of Terraform's state.
  For aws-sm, p0_aws_secrets_manager installed for the account that holds the secret. Give the connector's role secretsmanager:GetSecretValue on the secret alone, and kms:Decrypt on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.
  For gcp-sm, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone.
  Note: This integration is currently in beta.
  For the full install steps, see Installing LaunchDarkly https://docs.p0.dev/integrations/resource-integrations/launchdarkly/installation.
---

# p0_launchdarkly_account (Resource)

A LaunchDarkly account whose existing members P0 can grant base roles, teams, custom roles and role attributes to, through one of your LaunchDarkly connectors. Requests name the account by its `id`, as in `acme-prod/writer`.

P0 never holds the account's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector reaches the account, where the account is hosted, its plan, and where the token's secret is.

**Important:** Install the connector first, store the token, and grant the connector's identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the account through the connector: the connector reads the secret, signs in to LaunchDarkly with the token, checks the account's plan, and reads its own allowed email domains. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

P0 can hold each LaunchDarkly account only once. It refuses an account whose token is for a LaunchDarkly account it already has.

An account's attributes can't change once it is created. Changing `id`, `connector`, `instance`, `plan`, `vault` or `token_secret_name` replaces the account. To rotate the token, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the account from P0. P0 can't revoke access it granted on an account it no longer has, so that access stays in LaunchDarkly until you remove it there. Before you change an attribute or destroy this resource, revoke the account's open access in P0 or wait for it to expire.

An account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: `terraform apply -replace=<address>` deletes the account and creates it again, and `terraform state rm <address>` stops managing it.

The email domains that may receive access are set on the connector, as its `DOMAIN_ALLOW_PATTERN` environment variable, and aren't an attribute of this resource. P0 reads them back as `allowed_domains`.

**Prerequisites:**

- A LaunchDarkly connector, installed with `p0_launchdarkly_connector`.

- An access token for the account. On Enterprise, a service token with an inline policy that allows what P0 grants. On any other plan, the personal access token of a LaunchDarkly admin with the Admin role: P0 then grants base roles only, and the token stops working when that admin leaves LaunchDarkly.

- A secret that holds the token alone, as plain text, in the secret manager that `vault` names. Store it outside Terraform, which keeps the token out of Terraform's state.

- For `aws-sm`, `p0_aws_secrets_manager` installed for the account that holds the secret. Give the connector's role `secretsmanager:GetSecretValue` on the secret alone, and `kms:Decrypt` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For `gcp-sm`, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone.

**Note:** This integration is currently in beta.

For the full install steps, see [Installing LaunchDarkly](https://docs.p0.dev/integrations/resource-integrations/launchdarkly/installation).

## Example Usage

### Google Cloud

```terraform
# Adds a LaunchDarkly account that P0's LaunchDarkly connector on Google Cloud
# Run reaches, with its access token's secret in Google Secret Manager. For a
# connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_launchdarkly_connector example, which deploys and installs
# the connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the account's access token, as this resource's
# page describes: a service token on Enterprise, or an admin's personal token
# on any other plan. Store it, and nothing else, as the secret
# launchdarkly-acme-prod-token. The example doesn't create the secret, which
# keeps the token out of Terraform's state. To store it, with the token alone
# in token.txt and gcloud signed in to the secret's project:
#
#   gcloud secrets create launchdarkly-acme-prod-token --project my-secrets-project \
#     --data-file token.txt

locals {
  launchdarkly_account = {
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "launchdarkly-acme-prod-token"
  }
}

# The connector reads the access token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "launchdarkly_accessor" {
  project   = local.launchdarkly_account.secrets_project_id
  secret_id = local.launchdarkly_account.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.launchdarkly.email}"
}

resource "google_secret_manager_secret_iam_member" "launchdarkly_viewer" {
  project   = local.launchdarkly_account.secrets_project_id
  secret_id = local.launchdarkly_account.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.launchdarkly.email}"
}

# Creating it has P0 check the account through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_launchdarkly_account" "example" {
  # Requests name the account by this, as in acme-prod/writer.
  id        = "acme-prod"
  connector = p0_launchdarkly_connector.example.id
  # commercial, eu or federal.
  instance = "commercial"
  # enterprise, or non-enterprise for any other plan.
  plan = "enterprise"

  vault = {
    type       = "gcp-sm"
    project_id = local.launchdarkly_account.secrets_project_id
  }
  token_secret_name = local.launchdarkly_account.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.launchdarkly_accessor,
    google_secret_manager_secret_iam_member.launchdarkly_viewer,
  ]
}
```

### AWS

```terraform
# The AWS variant of the p0_launchdarkly_account example in this directory: a
# LaunchDarkly account that P0's LaunchDarkly connector on AWS Lambda reaches,
# with its access token's secret in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_launchdarkly_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the account's access token, as this resource's
# page describes, and store it, and nothing else, as the secret
# launchdarkly/acme-prod/token. To store it, with the token alone in token.txt
# and the AWS CLI signed in to the secret's account:
#
#   aws secretsmanager create-secret --name launchdarkly/acme-prod/token \
#     --secret-string file://token.txt --region us-east-1

locals {
  launchdarkly_account_aws = {
    secrets_region = "us-east-1"
    secret_name    = "launchdarkly/acme-prod/token"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "launchdarkly" {
  provider = aws.launchdarkly
}

# The connector reads this account's access token secret, and no other. Each
# account on the connector adds a policy of its own.
resource "aws_iam_role_policy" "launchdarkly_account" {
  provider = aws.launchdarkly

  name = "p0-launchdarkly-acme-prod-aws-read"
  role = aws_iam_role.launchdarkly.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadSecret"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # Secrets Manager adds a hyphen and six random characters to the name
        # in the secret's ARN, and "??????" matches those, so the grant gets
        # this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.launchdarkly_account_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.launchdarkly_account_aws.secret_name}-??????",
        ])
      }],
      local.launchdarkly_account_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.launchdarkly_account_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.launchdarkly_account_aws.secrets_region}.${data.aws_partition.launchdarkly.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the account through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_launchdarkly_account" "aws_example" {
  id        = "acme-prod-aws"
  connector = p0_launchdarkly_connector.aws_example.id
  instance  = "commercial"
  plan      = "enterprise"

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.launchdarkly_account_aws.secrets_region
  }
  token_secret_name = local.launchdarkly_account_aws.secret_name

  depends_on = [aws_iam_role_policy.launchdarkly_account]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The `id` of the `p0_launchdarkly_connector` that reaches this account. The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`.
- `id` (String) A name you choose for the account, such as `acme-prod`, which requests name it by: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters
- `instance` (String) Where your LaunchDarkly account is hosted: `commercial` (app.launchdarkly.com), `eu` (app.eu.launchdarkly.com) or `federal` (app.launchdarkly.us).
- `plan` (String) Your LaunchDarkly plan: `enterprise`, or `non-enterprise` for any other plan, such as Developer, Foundation or a legacy plan like Starter. On Enterprise, P0 grants base roles, teams, custom roles and role attributes with a service token. On any other plan, P0 grants base roles only, with an admin's personal token. P0 checks the plan when it validates the account. Changing it replaces the account.
- `token_secret_name` (String) The name of the secret that holds the account's LaunchDarkly access token, in `vault`: a service token on Enterprise, or an admin's personal token on any other plan. Only your connector reads it, never P0. The name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.
- `vault` (Attributes) The secret manager that holds the access token's secret, as installed in P0. Only the connector reads the secret, never P0, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `account_id` (String) The ID of the LaunchDarkly account the access token belongs to, which P0 reads through the connector when it validates the account.
- `allowed_domains` (String) The email domains the connector grants to: the `DOMAIN_ALLOW_PATTERN` it's deployed with, which P0 reads back from the connector when it validates the account. To change it, change the connector's deployment.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed
- `token_kind` (String) Whether the access token is a `service` token or a member's `personal` token, which P0 reads through the connector when it validates the account.
- `token_owner_member_id` (String) The ID of the LaunchDarkly member who owns a personal access token. P0 never grants to this member. Empty for a service token.

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0 (`p0_aws_secrets_manager`). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
