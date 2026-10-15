---
page_title: "p0_harbor_instance Resource - p0"
subcategory: ""
description: |-
  A Harbor instance that P0 can grant project roles on.
  P0 reaches the instance through a Harbor connector, which you install with p0_harbor_connector. The connector calls the instance as a robot account whose credentials stay in your own secret manager: P0 never reads them, and only the connector does. One connector can serve several instances, and each instance has its own robot account and secret.
  Important: Install the connector first, and grant its identity read access to the robot account's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.
  An instance's attributes can't change once it is created. Changing id, connector, vault, secret_name or admin_secret_name replaces the instance. A plan refuses a change to an installed instance's url: a user's ID on one Harbor instance means nothing on another, so for another address, add a new p0_harbor_instance with its own id, and remove this one once its open access has ended. To rotate the robot account's secret, add a new version to the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so that access stays in Harbor until you remove it there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.
  An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: terraform apply -replace=<address> deletes the instance and creates it again, and terraform state rm <address> stops managing it.
  The email domains that may receive access are set on the connector, as its ALLOWED_EMAIL_DOMAIN_PATTERN environment variable, and aren't an attribute of this resource.
  Prerequisites:
  
  Harbor 2.12.0 or later, the first version whose robot accounts can hold the permissions below.
  A robot account for P0's connector on the instance, created in Harbor under Administration, Robot Accounts, such as p0-connector. Give it the system permission User: List, and Member: List, Create, Update and Delete on all projects. Don't give it User: Read, which reads every OIDC user's CLI secret. Set it never to expire, and use it for nothing else: it can make anyone a member of any project at any role, Project Admin included.
  A secret that holds the file Harbor exports for the robot account, unchanged, in the secret manager that vault names. The connector reads the file's name and secret fields.
  For aws-sm, p0_aws_secrets_manager installed for the account that holds the secret. Give the connector's role secretsmanager:GetSecretValue on the secret alone, and kms:Decrypt on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.
  For gcp-sm, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone.
  Note: This integration is currently in beta.
  For the full install steps, see Installing Harbor https://docs.p0.dev/integrations/resource-integrations/harbor/installation.
---

# p0_harbor_instance (Resource)

A Harbor instance that P0 can grant project roles on.

P0 reaches the instance through a Harbor connector, which you install with `p0_harbor_connector`. The connector calls the instance as a robot account whose credentials stay in your own secret manager: P0 never reads them, and only the connector does. One connector can serve several instances, and each instance has its own robot account and secret.

**Important:** Install the connector first, and grant its identity read access to the robot account's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.

An instance's attributes can't change once it is created. Changing `id`, `connector`, `vault`, `secret_name` or `admin_secret_name` replaces the instance. A plan refuses a change to an installed instance's `url`: a user's ID on one Harbor instance means nothing on another, so for another address, add a new `p0_harbor_instance` with its own `id`, and remove this one once its open access has ended. To rotate the robot account's secret, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so that access stays in Harbor until you remove it there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.

An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: `terraform apply -replace=<address>` deletes the instance and creates it again, and `terraform state rm <address>` stops managing it.

The email domains that may receive access are set on the connector, as its `ALLOWED_EMAIL_DOMAIN_PATTERN` environment variable, and aren't an attribute of this resource.

**Prerequisites:**

- Harbor 2.12.0 or later, the first version whose robot accounts can hold the permissions below.

- A robot account for P0's connector on the instance, created in Harbor under Administration, Robot Accounts, such as `p0-connector`. Give it the system permission User: List, and Member: List, Create, Update and Delete on all projects. Don't give it User: Read, which reads every OIDC user's CLI secret. Set it never to expire, and use it for nothing else: it can make anyone a member of any project at any role, Project Admin included.

- A secret that holds the file Harbor exports for the robot account, unchanged, in the secret manager that `vault` names. The connector reads the file's `name` and `secret` fields.

- For `aws-sm`, `p0_aws_secrets_manager` installed for the account that holds the secret. Give the connector's role `secretsmanager:GetSecretValue` on the secret alone, and `kms:Decrypt` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For `gcp-sm`, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone.

**Note:** This integration is currently in beta.

For the full install steps, see [Installing Harbor](https://docs.p0.dev/integrations/resource-integrations/harbor/installation).

## Example Usage

### Google Cloud

```terraform
# Adds a Harbor instance that P0's Harbor connector on Google Cloud Run reaches,
# with the robot account's secret in Google Secret Manager. For a connector on
# AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_harbor_connector example, which deploys and installs the
# connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the connector's robot account in Harbor, as this
# resource's page describes, and store the file Harbor exports for it as the
# secret harbor-prod-robot. The example doesn't create the secret, which keeps
# the robot account's credentials out of Terraform's state. To store it, with
# gcloud signed in to the secret's project:
#
#   gcloud secrets create harbor-prod-robot --project my-secrets-project \
#     --data-file 'robot$p0-connector.json'

locals {
  harbor_instance = {
    # The instance's address: https and the host, with nothing after it.
    url = "https://harbor.example.com"
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "harbor-prod-robot"
    # Optional: the secret that holds a Harbor admin credential, for the system
    # administrator access. Store it like the robot's, as a JSON object with
    # issuer, clientId, clientSecret and refreshToken for a dedicated identity
    # provider user who is a Harbor system administrator. It is as sensitive as
    # an admin password.
    admin_secret_name = "harbor-prod-admin"
  }
}

# The connector reads the robot account's secret, and no other. It reads the
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

# The same two roles on the admin credential's secret.
resource "google_secret_manager_secret_iam_member" "harbor_admin_accessor" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.admin_secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

resource "google_secret_manager_secret_iam_member" "harbor_admin_viewer" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.admin_secret_name
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
  secret_name       = local.harbor_instance.secret_name
  admin_secret_name = local.harbor_instance.admin_secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.harbor_accessor,
    google_secret_manager_secret_iam_member.harbor_viewer,
    google_secret_manager_secret_iam_member.harbor_admin_accessor,
    google_secret_manager_secret_iam_member.harbor_admin_viewer,
  ]
}
```

### AWS

```terraform
# The AWS variant of the p0_harbor_instance example in this directory: a Harbor
# instance that P0's Harbor connector on AWS Lambda reaches, with the robot
# account's secret in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_harbor_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the connector's robot account in Harbor, as
# this resource's page describes, and store the file Harbor exports for it as
# the secret harbor/prod/robot. To store it, with the AWS CLI signed in to the
# secret's account:
#
#   aws secretsmanager create-secret --name harbor/prod/robot \
#     --secret-string 'file://robot$p0-connector.json' --region us-east-1

locals {
  harbor_instance_aws = {
    url            = "https://harbor.example.com"
    secrets_region = "us-east-1"
    secret_name    = "harbor/prod/robot"
    # Optional: the secret that holds a Harbor admin credential, for the system
    # administrator access, in the same account and region. It is as sensitive
    # as an admin password.
    admin_secret_name = "harbor/prod/admin"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "harbor" {
  provider = aws.harbor
}

# The connector reads the robot account's secret and the admin credential's,
# and no other.
resource "aws_iam_role_policy" "harbor_robot_secret" {
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
        # in a secret's ARN, and "??????" matches those, so the grant gets
        # these two secrets and no other.
        Resource = [
          for name in [local.harbor_instance_aws.secret_name, local.harbor_instance_aws.admin_secret_name] :
          join(":", [
            "arn:aws:secretsmanager",
            local.harbor_instance_aws.secrets_region,
            p0_aws_secrets_manager.example.account_id,
            "secret",
            "${name}-??????",
          ])
        ]
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
  secret_name       = local.harbor_instance_aws.secret_name
  admin_secret_name = local.harbor_instance_aws.admin_secret_name

  depends_on = [aws_iam_role_policy.harbor_robot_secret]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The `id` of the `p0_harbor_connector` that reaches this instance. The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`.
- `id` (String) A name you choose for the instance, such as `prod`: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters
- `secret_name` (String) The name of the secret that holds the robot account's credentials, in `vault`: the name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.
- `url` (String) The address your Harbor instance is served at, such as `https://harbor.example.com`: `https://` and the host, with a port if it isn't 443, in lowercase and with nothing after it. Leave out a trailing slash, any path, and any username or password. P0 stores the address in this spelling alone, and the connector sends the robot account's credentials to it. It can't change: for another address, add a new `p0_harbor_instance` with its own `id`.
- `vault` (Attributes) The secret manager that holds the connector robot account's secret, as installed in P0. P0 never reads the secret. The connector does, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Optional

- `admin_secret_name` (String) Optional. The name of the secret that holds a Harbor admin credential, in `vault` beside the robot account's secret: the name alone, never an ARN or a resource name, under the same rules as `secret_name`. Setting it lets P0 grant Harbor's system administrator access on this instance, which a robot account can't do. The secret holds a JSON object, `{ "issuer", "clientId", "clientSecret", "refreshToken" }`, for a dedicated user in your identity provider who is a Harbor system administrator. It is as sensitive as an admin password: give the connector, and only the connector, read access to it, as for the robot account's secret. P0 never reads it. Install validation also checks it. Leave it unset for no system administrator access. Changing it replaces the instance.

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
