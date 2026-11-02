---
page_title: "p0_miro_ai_team Resource - p0"
subcategory: ""
description: |-
  A Miro team whose boards P0 can share with the team's members, through one of your Miro (AI) connectors. Requests name the team by its id, as in board design <board-id> viewer.
  P0 never holds the connector user's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector reaches the team, and where the token's secret is.
  Important: Install the connector first, store the token, and grant the connector's identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the team through the connector: the connector reads the secret, checks the token's team and its scopes with Miro, and counts the boards the connector user owns or co-owns. If a check fails, the apply fails with P0's message. Zero shareable boards doesn't fail the check, but nothing can be requested until the connector user owns or co-owns a board. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.
  P0 can hold each Miro team only once. It refuses a team whose token is for a Miro team it already has.
  A team's attributes can't change once it is created. Changing id, connector, vault or token_secret_name replaces the team. To rotate the token, add a new version to the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the team from P0. P0 can't revoke access it granted on a team it no longer has, so that access stays in Miro until you remove it there. Before you change an attribute or destroy this resource, revoke the team's open access in P0 or wait for it to expire.
  A team that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported team that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: terraform apply -replace=<address> deletes the team and creates it again, and terraform state rm <address> stops managing it.
  Prerequisites:
  
  A Miro (AI) connector, installed with p0_miro_ai_connector. Apply the connector before the team.
  A connector user in the Miro team, and an access token for it: from a private Miro app, created as the connector user, with the boards:read, boards:write and team:read scopes and non-expiring tokens ("Install app and get OAuth token"). A token keeps the scopes it was issued with, so adding a scope later means installing the app again and storing the new token.
  The connector user as owner or co-owner of each board that should be requestable.
  A secret that holds the token alone, as plain text, in the secret manager that vault names. Store it outside Terraform, which keeps the token out of Terraform's state.
  For aws-sm, p0_aws_secrets_manager installed for the account that holds the secret. Give the connector's role secretsmanager:GetSecretValue on the secret alone, and kms:Decrypt on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.
  For gcp-sm, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone.
  Note: This integration is currently in beta.
  For the full install steps, see Miro (AI) https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/miro.
---

# p0_miro_ai_team (Resource)

A Miro team whose boards P0 can share with the team's members, through one of your Miro (AI) connectors. Requests name the team by its `id`, as in `board design <board-id> viewer`.

P0 never holds the connector user's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector reaches the team, and where the token's secret is.

**Important:** Install the connector first, store the token, and grant the connector's identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the team through the connector: the connector reads the secret, checks the token's team and its scopes with Miro, and counts the boards the connector user owns or co-owns. If a check fails, the apply fails with P0's message. Zero shareable boards doesn't fail the check, but nothing can be requested until the connector user owns or co-owns a board. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

P0 can hold each Miro team only once. It refuses a team whose token is for a Miro team it already has.

A team's attributes can't change once it is created. Changing `id`, `connector`, `vault` or `token_secret_name` replaces the team. To rotate the token, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the team from P0. P0 can't revoke access it granted on a team it no longer has, so that access stays in Miro until you remove it there. Before you change an attribute or destroy this resource, revoke the team's open access in P0 or wait for it to expire.

A team that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported team that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: `terraform apply -replace=<address>` deletes the team and creates it again, and `terraform state rm <address>` stops managing it.

**Prerequisites:**

- A Miro (AI) connector, installed with `p0_miro_ai_connector`. Apply the connector before the team.

- A connector user in the Miro team, and an access token for it: from a private Miro app, created as the connector user, with the `boards:read`, `boards:write` and `team:read` scopes and non-expiring tokens ("Install app and get OAuth token"). A token keeps the scopes it was issued with, so adding a scope later means installing the app again and storing the new token.

- The connector user as owner or co-owner of each board that should be requestable.

- A secret that holds the token alone, as plain text, in the secret manager that `vault` names. Store it outside Terraform, which keeps the token out of Terraform's state.

- For `aws-sm`, `p0_aws_secrets_manager` installed for the account that holds the secret. Give the connector's role `secretsmanager:GetSecretValue` on the secret alone, and `kms:Decrypt` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For `gcp-sm`, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone.

**Note:** This integration is currently in beta.

For the full install steps, see [Miro (AI)](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/miro).

## Example Usage

### Google Cloud

```terraform
# Adds a Miro team that P0's Miro (AI) connector on Google Cloud Run reaches,
# with the connector user's access token secret in Google Secret Manager. For a
# connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_miro_ai_connector example, which deploys and installs the
# connector. Apply the connector before the team. Google Secret Manager must
# already be installed in P0 for the secret's project: this provider has no
# resource for it, so install it in the P0 app.
#
# Before you apply this, create the connector user's access token, as this
# resource's page describes: from a private Miro app with the boards:read,
# boards:write and team:read scopes. Store it, and nothing else, as the secret
# miro-design-token. The example doesn't create the secret, which keeps the
# token out of Terraform's state. To store it, with the token alone in
# token.txt and gcloud signed in to the secret's project:
#
#   gcloud secrets create miro-design-token --project my-secrets-project \
#     --data-file token.txt

locals {
  miro_ai_team = {
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "miro-design-token"
  }
}

# The connector reads the access token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "miro_ai_accessor" {
  project   = local.miro_ai_team.secrets_project_id
  secret_id = local.miro_ai_team.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.miro_ai.email}"
}

resource "google_secret_manager_secret_iam_member" "miro_ai_viewer" {
  project   = local.miro_ai_team.secrets_project_id
  secret_id = local.miro_ai_team.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.miro_ai.email}"
}

# Creating it has P0 check the team through the connector, so it waits until
# the connector is installed and the grants exist. IAM changes can take a few
# seconds to take effect, so if the first check is denied access, apply again.
resource "p0_miro_ai_team" "example" {
  # Requests name the team by this, as in board design <board-id> viewer.
  id        = "design"
  connector = p0_miro_ai_connector.example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.miro_ai_team.secrets_project_id
  }
  token_secret_name = local.miro_ai_team.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.miro_ai_accessor,
    google_secret_manager_secret_iam_member.miro_ai_viewer,
  ]
}
```

### AWS

```terraform
# The AWS variant of the p0_miro_ai_team example in this directory: a
# Miro team that P0's Miro (AI) connector on AWS Lambda reaches, with the
# connector user's access token secret in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_miro_ai_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the connector user's access token, as this
# resource's page describes, and store it, and nothing else, as the secret
# miro/design/token. To store it, with the token alone in token.txt
# and the AWS CLI signed in to the secret's account:
#
#   aws secretsmanager create-secret --name miro/design/token \
#     --secret-string file://token.txt --region us-east-1

locals {
  miro_ai_team_aws = {
    secrets_region = "us-east-1"
    secret_name    = "miro/design/token"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "miro_ai" {
  provider = aws.miro_ai
}

# The connector reads this team's access token secret, and no other. Each
# team on the connector adds a policy of its own.
resource "aws_iam_role_policy" "miro_ai_team" {
  provider = aws.miro_ai

  name = "p0-miro-ai-design-aws-read"
  role = aws_iam_role.miro_ai.id
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
          local.miro_ai_team_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.miro_ai_team_aws.secret_name}-??????",
        ])
      }],
      local.miro_ai_team_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.miro_ai_team_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.miro_ai_team_aws.secrets_region}.${data.aws_partition.miro_ai.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the team through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_miro_ai_team" "aws_example" {
  # Requests name the team by this, as in board design-aws <board-id> viewer.
  id        = "design-aws"
  connector = p0_miro_ai_connector.aws_example.id

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.miro_ai_team_aws.secrets_region
  }
  token_secret_name = local.miro_ai_team_aws.secret_name

  depends_on = [aws_iam_role_policy.miro_ai_team]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The `id` of the `p0_miro_ai_connector` that reaches this team. The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`.
- `id` (String) A name you choose for the team, such as `design`, which requests name it by: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters
- `token_secret_name` (String) The name of the secret that holds the connector user's Miro access token, in `vault`. Only your connector reads it, never P0. The name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.
- `vault` (Attributes) The secret manager that holds the access token's secret, as installed in P0. Only the connector reads the secret, never P0, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `shareable_boards` (String) How many of the team's boards the connector user owns or co-owns, and so P0 can share, as a string. P0 reads it through the connector when it validates the team.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed
- `team_id` (String) The ID of the Miro team the access token was installed into, which P0 reads through the connector when it validates the team.

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0 (`p0_aws_secrets_manager`). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
