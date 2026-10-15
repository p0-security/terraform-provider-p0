---
page_title: "p0_harbor_instance Resource - p0"
subcategory: ""
description: |-
  A Harbor instance that P0 can grant project roles, and system administrator, on.
  P0 reaches the instance through a Harbor connector, which you install with p0_harbor_connector. The connector signs in to the instance as a Harbor admin account whose OIDC credential stays in your own secret manager: P0 never reads it, and only the connector does. One connector can serve several instances, and each instance has its own admin credential and secret.
  Important: Install the connector first, and grant its identity read access to the admin credential's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.
  An instance's attributes can't change once it is created. Changing id, connector, vault or secret_name replaces the instance. A plan refuses a change to an installed instance's url: a user's ID on one Harbor instance means nothing on another, so for another address, add a new p0_harbor_instance with its own id, and remove this one once its open access has ended. To rotate the admin credential, add a new version to the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so that access stays in Harbor until you remove it there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.
  An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: terraform apply -replace=<address> deletes the instance and creates it again, and terraform state rm <address> stops managing it.
  The email domains that may receive access are set on the connector, as its ALLOWED_EMAIL_DOMAIN_PATTERN environment variable, and aren't an attribute of this resource.
  Prerequisites:
  
  Harbor signing users in with OIDC.
  A dedicated user in your identity provider who is a Harbor system administrator, used for nothing else. The connector signs in as this user, so it can make anyone a member of any project at any role, and a system administrator.
  A secret that holds that user's OIDC credential, in the secret manager that vault names: JSON with issuer, clientId, clientSecret and refreshToken. It is as sensitive as an admin password.
  For aws-sm, p0_aws_secrets_manager installed for the account that holds the secret. Give the connector's role secretsmanager:GetSecretValue on the secret alone, and kms:Decrypt on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.
  For gcp-sm, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone.
  Note: This integration is currently in beta.
  For the full install steps, see Installing Harbor https://docs.p0.dev/integrations/resource-integrations/harbor/installation.
---

# p0_harbor_instance (Resource)

A Harbor instance that P0 can grant project roles, and system administrator, on.

P0 reaches the instance through a Harbor connector, which you install with `p0_harbor_connector`. The connector signs in to the instance as a Harbor admin account whose OIDC credential stays in your own secret manager: P0 never reads it, and only the connector does. One connector can serve several instances, and each instance has its own admin credential and secret.

**Important:** Install the connector first, and grant its identity read access to the admin credential's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.

An instance's attributes can't change once it is created. Changing `id`, `connector`, `vault` or `secret_name` replaces the instance. A plan refuses a change to an installed instance's `url`: a user's ID on one Harbor instance means nothing on another, so for another address, add a new `p0_harbor_instance` with its own `id`, and remove this one once its open access has ended. To rotate the admin credential, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so that access stays in Harbor until you remove it there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.

An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: `terraform apply -replace=<address>` deletes the instance and creates it again, and `terraform state rm <address>` stops managing it.

The email domains that may receive access are set on the connector, as its `ALLOWED_EMAIL_DOMAIN_PATTERN` environment variable, and aren't an attribute of this resource.

**Prerequisites:**

- Harbor signing users in with OIDC.

- A dedicated user in your identity provider who is a Harbor system administrator, used for nothing else. The connector signs in as this user, so it can make anyone a member of any project at any role, and a system administrator.

- A secret that holds that user's OIDC credential, in the secret manager that `vault` names: JSON with `issuer`, `clientId`, `clientSecret` and `refreshToken`. It is as sensitive as an admin password.

- For `aws-sm`, `p0_aws_secrets_manager` installed for the account that holds the secret. Give the connector's role `secretsmanager:GetSecretValue` on the secret alone, and `kms:Decrypt` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For `gcp-sm`, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone.

**Note:** This integration is currently in beta.

For the full install steps, see [Installing Harbor](https://docs.p0.dev/integrations/resource-integrations/harbor/installation).

## Example Usage

### Google Cloud

```terraform
# Adds a Harbor instance that P0's Harbor connector on Google Cloud Run reaches,
# with its admin credential's secret in Google Secret Manager. For a connector on
# AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_harbor_connector example, which deploys and installs the
# connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, set up the Harbor admin account the connector signs in
# as, as this resource's page describes, and store its OIDC credential as the
# secret harbor-prod-admin: JSON with issuer, clientId, clientSecret and
# refreshToken. The example doesn't create the secret, which keeps the
# credential out of Terraform's state. To store it, with gcloud signed in to the
# secret's project:
#
#   gcloud secrets create harbor-prod-admin --project my-secrets-project \
#     --data-file harbor-admin.json

locals {
  harbor_instance = {
    # The instance's address: https and the host, with nothing after it.
    url = "https://harbor.example.com"
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "harbor-prod-admin"
  }
}

# The connector reads the admin credential's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "harbor_accessor" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

resource "google_secret_manager_secret_iam_member" "harbor_viewer" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

# Creating it has P0 check the instance through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_harbor_instance" "example" {
  id        = "prod"
  connector = p0_harbor_connector.example.id
  url       = local.harbor_instance.url

  vault = {
    type       = "gcp-sm"
    project_id = local.harbor_instance.secrets_project_id
  }
  secret_name = local.harbor_instance.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.harbor_accessor,
    google_secret_manager_secret_iam_member.harbor_viewer,
  ]
}
```

### AWS

```terraform
# The AWS variant of the p0_harbor_instance example in this directory: a Harbor
# instance that P0's Harbor connector on AWS Lambda reaches, with its admin
# credential's secret in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_harbor_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, set up the Harbor admin account the connector signs in
# as, as this resource's page describes, and store its OIDC credential as the
# secret harbor/prod/admin: JSON with issuer, clientId, clientSecret and
# refreshToken. To store it, with the AWS CLI signed in to the secret's account:
#
#   aws secretsmanager create-secret --name harbor/prod/admin \
#     --secret-string file://harbor-admin.json --region us-east-1

locals {
  harbor_instance_aws = {
    url            = "https://harbor.example.com"
    secrets_region = "us-east-1"
    secret_name    = "harbor/prod/admin"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "harbor" {
  provider = aws.harbor
}

# The connector reads the admin credential's secret, and no other.
resource "aws_iam_role_policy" "harbor_admin_secret" {
  provider = aws.harbor

  name = "p0-harbor-prod-aws-read"
  role = aws_iam_role.harbor.id
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
          local.harbor_instance_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.harbor_instance_aws.secret_name}-??????",
        ])
      }],
      local.harbor_instance_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.harbor_instance_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.harbor_instance_aws.secrets_region}.${data.aws_partition.harbor.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the instance through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_harbor_instance" "aws_example" {
  id        = "prod-aws"
  connector = p0_harbor_connector.aws_example.id
  url       = local.harbor_instance_aws.url

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.harbor_instance_aws.secrets_region
  }
  secret_name = local.harbor_instance_aws.secret_name

  depends_on = [aws_iam_role_policy.harbor_admin_secret]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The `id` of the `p0_harbor_connector` that reaches this instance. The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`.
- `id` (String) A name you choose for the instance, such as `prod`: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters
- `secret_name` (String) The name of the secret that holds the OIDC credential of a Harbor admin account, in the vault above: JSON with "issuer", "clientId", "clientSecret" and "refreshToken". Harbor has to sign users in with OIDC. P0 never reads it; only your connector does. The name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.
- `url` (String) The address your Harbor instance is served at, such as `https://harbor.example.com`: `https://` and the host, with a port if it isn't 443, in lowercase and with nothing after it. Leave out a trailing slash, any path, and any username or password. P0 stores the address in this spelling alone, and the connector signs in to it with the admin credential. It can't change: for another address, add a new `p0_harbor_instance` with its own `id`.
- `vault` (Attributes) The secret manager that holds the admin credential's secret, as installed in P0. P0 never reads the secret. The connector does, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0 (`p0_aws_secrets_manager`). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
