---
page_title: "p0_pingone_aws_iam_federation_app Resource - p0"
subcategory: ""
description: |-
  A PingOne SAML app that signs users in to AWS, and that P0 assigns AWS roles in through one of your PingOne (AWS IAM Federation) connectors. A p0_aws_iam_federation resource has an AWS account trust it.
  The connector acts on the environment as a PingOne worker app, which signs in with a client secret. P0 never holds the client secret. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the environment, the role attribute, the worker app's client ID, and where the client secret is.
  Important: Create the worker app, store its client secret and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to PingOne as the worker app, and checks the SAML app and the role attribute. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.
  P0 fixes every attribute when it adds the app, so changing any of them replaces the installation. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.
  Prerequisites:
  
  A PingOne (AWS IAM Federation) connector, installed with p0_pingone_aws_iam_federation_connector. The client secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.
  A multi-valued PingOne user attribute, named by role_attribute, that P0 adds each user's AWS roles to.
  The PingOne SAML app. It maps role_attribute to https://aws.amazon.com/SAML/Attributes/Role, and also sends https://aws.amazon.com/SAML/Attributes/RoleSessionName and https://aws.amazon.com/SAML/Attributes/SourceIdentity, each with the URI name format. Its subject uses the persistent NameID format.
  A PingOne worker app in the same environment, whose roles let it read applications and their attribute mappings, read the user schema, and read and update users. Store its client secret outside Terraform, as the secret's whole value, in the account and region or the project that vault names. Storing it outside Terraform keeps it out of Terraform's state.
  Read access to that one secret, and no other, for the connector's identity (its connector_identity). P0 names the secret in each call, so a call could point the connector at any other secret it can read.
  On AWS, give the connector's role secretsmanager:GetSecretValue on arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ?????? matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role kms:Decrypt on that key, with the condition that kms:ViaService is secretsmanager.<secrets_region>.amazonaws.com. A secret outside the connector's account needs a customer-managed key, because no other account can use the default aws/secretsmanager key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.
  On Google Cloud, give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret itself, not on its project.
  P0's secret manager integration for where the secret is: AWS Secrets Manager (p0_aws_secrets_manager) for the AWS account, or Google Secret Manager for the Google Cloud project.
  Note: This integration is currently in preview.
  For the AWS install steps, see Setting up AWS IAM management https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods.
---

# p0_pingone_aws_iam_federation_app (Resource)

A PingOne SAML app that signs users in to AWS, and that P0 assigns AWS roles in through one of your PingOne (AWS IAM Federation) connectors. A `p0_aws_iam_federation` resource has an AWS account trust it.

The connector acts on the environment as a PingOne worker app, which signs in with a client secret. P0 never holds the client secret. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the environment, the role attribute, the worker app's client ID, and where the client secret is.

**Important:** Create the worker app, store its client secret and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to PingOne as the worker app, and checks the SAML app and the role attribute. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes every attribute when it adds the app, so changing any of them replaces the installation. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- A PingOne (AWS IAM Federation) connector, installed with `p0_pingone_aws_iam_federation_connector`. The client secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A multi-valued PingOne user attribute, named by `role_attribute`, that P0 adds each user's AWS roles to.

- The PingOne SAML app. It maps `role_attribute` to `https://aws.amazon.com/SAML/Attributes/Role`, and also sends `https://aws.amazon.com/SAML/Attributes/RoleSessionName` and `https://aws.amazon.com/SAML/Attributes/SourceIdentity`, each with the URI name format. Its subject uses the persistent NameID format.

- A PingOne worker app in the same environment, whose roles let it read applications and their attribute mappings, read the user schema, and read and update users. Store its client secret outside Terraform, as the secret's whole value, in the account and region or the project that `vault` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its `connector_identity`). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role `secretsmanager:GetSecretValue` on `arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and `??????` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role `kms:Decrypt` on that key, with the condition that `kms:ViaService` is `secretsmanager.<secrets_region>.amazonaws.com`. A secret outside the connector's account needs a customer-managed key, because no other account can use the default `aws/secretsmanager` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (`p0_aws_secrets_manager`) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).

## Example Usage

### AWS

```terraform
# Adds the PingOne SAML app 4f2a9c1e-8b3d-4e6f-a1c2-9d8e7f6a5b4c, whose worker app's client secret is
# in AWS Secrets Manager, in the account of the connector on AWS Lambda that the
# p0_pingone_aws_iam_federation_connector example installs. For a connector on
# Cloud Run and a client secret in Google Secret Manager, see gcp.tf in this
# directory.
#
# Before you apply this, in the PingOne environment that holds the SAML app:
#
# - Add a multi-valued user attribute, such as awsRoles, that P0 adds each
#   user's AWS roles to.
# - Map it in the SAML app to https://aws.amazon.com/SAML/Attributes/Role, and
#   also send https://aws.amazon.com/SAML/Attributes/RoleSessionName and
#   https://aws.amazon.com/SAML/Attributes/SourceIdentity, each with the URI
#   name format. Use the persistent NameID format for the subject.
# - Create a worker app whose roles let it read applications and their
#   attribute mappings, read the user schema, and read and update users. Copy
#   its client secret to the file pingone-client-secret.txt.
#
# The example doesn't create the secret, which keeps the client secret out of
# Terraform's state. Creating p0_pingone_aws_iam_federation_app has the
# connector read it, so store it before then, as the secret's whole value.
# With the AWS CLI signed in to the secret's account, run this where
# pingone-client-secret.txt is:
#
#   aws secretsmanager create-secret --name pingone/acme/p0 \
#     --secret-string file://pingone-client-secret.txt --region us-west-2
#
# If the secret exists, store the client secret as a new version instead,
# which is also how to rotate it:
#
#   aws secretsmanager put-secret-value --secret-id pingone/acme/p0 \
#     --secret-string file://pingone-client-secret.txt --region us-west-2
#
# Then delete pingone-client-secret.txt.
#
# The grant is what P0's PingOne (AWS IAM Federation) installer generates for
# the app, with p0_pingone_aws_iam_federation_app after it.

locals {
  p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the worker app's client secret: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the client
    # secret, add a new version to this secret instead.
    client_secret_name = "pingone/acme/p0"
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
  alias               = "p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c"
  region              = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_region
  allowed_account_ids = [local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.connector_account_id]
}

data "aws_partition" "p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c" {
  provider = aws.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c
}

# The connector reads the app's client secret, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity. Each app adds a policy of its own to that role, and a
# role's inline policies share a limit of 10,240 characters: past a few dozen
# apps, deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c_secret" {
  provider = aws.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c

  name = "ReadPingOneClientSecret-4f2a9c1e-8b3d-4e6f-a1c2-9d8e7f6a5b4c"
  role = p0_pingone_aws_iam_federation_connector.example.connector_identity.role_name
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
          local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_region,
          local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_account_id,
          "secret",
          "${local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.client_secret_name}-??????",
        ])
      }],
      local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.kms_key_arn == null ? [] : [{
        Sid      = "DecryptClientSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_region}.${data.aws_partition.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_account_id == local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.connector_account_id || local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.kms_key_arn != null
      error_message = "The client secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the client secret. IAM
# changes can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_pingone_aws_iam_federation_app" "example" {
  id             = "4f2a9c1e-8b3d-4e6f-a1c2-9d8e7f6a5b4c"
  connector      = p0_pingone_aws_iam_federation_connector.example.id
  region         = "NA"
  environment_id = "9c8b7a6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
  role_attribute = "awsRoles"

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_account_id
    secrets_region = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.secrets_region
  }
  # The worker app's client ID, from its Configuration tab.
  client_id          = "7d6c5b4a-3f2e-4d1c-9b8a-7f6e5d4c3b2a"
  client_secret_name = local.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c.client_secret_name

  depends_on = [aws_iam_role_policy.p0_pingone_aws_iam_federation_app_4f2a9c1e_8b3d_4e6f_a1c2_9d8e7f6a5b4c_secret]
}

# The app's SAML metadata, which each AWS account's SAML provider trusts (see
# the p0_aws_iam_federation example).
output "pingone_saml_metadata_url" {
  value = p0_pingone_aws_iam_federation_app.example.saml_metadata_url
}
```

### Google Cloud

```terraform
# The Google Cloud variant of the p0_pingone_aws_iam_federation_app example in
# this directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_pingone_aws_iam_federation_connector example), and its worker app's client
# secret is in Google Secret Manager.
#
# Before you apply this, set up the PingOne environment, as the example in
# resource.tf says, and store the worker app's client secret as the secret
# pingone-acme-eu-p0. The example doesn't create the secret, which keeps the
# client secret out of Terraform's state. Run this where
# pingone-client-secret.txt is:
#
#   gcloud secrets create pingone-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=pingone-client-secret.txt
#
# If the secret exists, store the client secret as a new version instead,
# which is also how to rotate it:
#
#   gcloud secrets versions add pingone-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=pingone-client-secret.txt
#
# Then delete pingone-client-secret.txt.

locals {
  p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b = {
    # The secret that holds the worker app's client secret, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the client secret, add a new version to this secret instead.
    client_secret_project = "my-secrets-project-id"
    client_secret_id      = "pingone-acme-eu-p0"
  }
}

# The connector reads the client secret's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_accessor" {
  project   = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  secret_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_pingone_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_viewer" {
  project   = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  secret_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_pingone_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the client secret.
resource "p0_pingone_aws_iam_federation_app" "gcp_example" {
  id             = "8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b"
  connector      = p0_pingone_aws_iam_federation_connector.gcp_example.id
  region         = "EU"
  environment_id = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
  role_attribute = "awsRoles"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  }
  # The worker app's client ID, from its Configuration tab.
  client_id          = "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"
  client_secret_name = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_accessor,
    google_secret_manager_secret_iam_member.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `client_id` (String) The client ID of the PingOne worker app that the connector signs in as
- `client_secret_name` (String) The name of the secret that holds the worker app's client secret, such as `pingone/acme/p0` in AWS Secrets Manager or `pingone-acme-p0` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like `-AbCdEf`, because Secrets Manager can't find a secret by such a name. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here.
- `connector` (String) The connector that acts on this app: the `id` of a `p0_pingone_aws_iam_federation_connector` resource. Requires that connector to be installed.
- `environment_id` (String) The ID of the PingOne environment that holds the app
- `id` (String) The ID of the PingOne SAML app that signs users in to AWS, as its Configuration tab in the PingOne admin console shows it
- `region` (String) The region of the PingOne environment: `NA`, `EU`, `CA`, `AP` or `AU`
- `role_attribute` (String) The name of the multi-valued user attribute that holds each user's AWS roles, such as `awsRoles`
- `vault` (Attributes) The secret manager that holds the client secret. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `saml_metadata_url` (String) The URL of the app's SAML metadata. Create each AWS account's SAML provider from it.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: either `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the client secret (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the client secret (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the client secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
