---
page_title: "p0_okta_aws_iam_federation_app Resource - p0"
subcategory: ""
description: |-
  An Okta AWS Account Federation app that P0 assigns AWS roles in, through one of your Okta (AWS IAM Federation) connectors. Users sign in to AWS through the app, and a p0_aws_iam_federation resource has an AWS account trust it.
  The connector acts on the org as an Okta API service app, which signs in with a private key. P0 never holds the private key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the org's domain, the service app's client ID, and where the private key is.
  Important: Create the service app, store its private key and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the key, signs in to Okta as the service app, and reads the federation app. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.
  P0 fixes the connector, the domain, the client ID, the vault and the secret when it adds the app, so changing any of them, or id, replaces the installation. To rotate the private key, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.
  Prerequisites:
  
  An Okta (AWS IAM Federation) connector, installed with p0_okta_aws_iam_federation_connector. The private key must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.
  The Okta AWS Account Federation app, with the https://aws.amazon.com/SAML/Attributes/SourceIdentity attribute. An app that a legacy federated AWS login uses can't be used.
  An Okta API service app that signs in with a public and private key, with these Okta API scopes and no other: okta.apps.manage, okta.schemas.manage and okta.users.read. Give it a custom admin role with the permissions Edit users' application assignments and Manage applications, on a resource set that holds all users and the federation app. Store its private key, in JSON format, outside Terraform, as the secret's whole value, in the account and region or the project that vault names. Storing it outside Terraform keeps it out of Terraform's state.
  Read access to that one secret, and no other, for the connector's identity (its connector_identity). P0 names the secret in each call, so a call could point the connector at any other secret it can read.
  On AWS, give the connector's role secretsmanager:GetSecretValue on arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ?????? matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role kms:Decrypt on that key, with the condition that kms:ViaService is secretsmanager.<secrets_region>.amazonaws.com. A secret outside the connector's account needs a customer-managed key, because no other account can use the default aws/secretsmanager key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.
  On Google Cloud, give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret itself, not on its project.
  P0's secret manager integration for where the secret is: AWS Secrets Manager (p0_aws_secrets_manager) for the AWS account, or Google Secret Manager for the Google Cloud project.
  Okta reads each AWS account's roles as an IAM user with iam:ListAccountAliases and iam:ListRoles. Create one in each account that trusts the app, give its access key to the app's provisioning settings, and refresh the app's data after the pool roles exist.
  Note: This integration is currently in preview.
  For the AWS install steps, see Setting up AWS IAM management https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods.
---

# p0_okta_aws_iam_federation_app (Resource)

An Okta AWS Account Federation app that P0 assigns AWS roles in, through one of your Okta (AWS IAM Federation) connectors. Users sign in to AWS through the app, and a `p0_aws_iam_federation` resource has an AWS account trust it.

The connector acts on the org as an Okta API service app, which signs in with a private key. P0 never holds the private key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the org's domain, the service app's client ID, and where the private key is.

**Important:** Create the service app, store its private key and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the key, signs in to Okta as the service app, and reads the federation app. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes the connector, the domain, the client ID, the vault and the secret when it adds the app, so changing any of them, or `id`, replaces the installation. To rotate the private key, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- An Okta (AWS IAM Federation) connector, installed with `p0_okta_aws_iam_federation_connector`. The private key must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- The Okta AWS Account Federation app, with the `https://aws.amazon.com/SAML/Attributes/SourceIdentity` attribute. An app that a legacy federated AWS login uses can't be used.

- An Okta API service app that signs in with a public and private key, with these Okta API scopes and no other: `okta.apps.manage`, `okta.schemas.manage` and `okta.users.read`. Give it a custom admin role with the permissions **Edit users' application assignments** and **Manage applications**, on a resource set that holds all users and the federation app. Store its private key, in JSON format, outside Terraform, as the secret's whole value, in the account and region or the project that `vault` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its `connector_identity`). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role `secretsmanager:GetSecretValue` on `arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and `??????` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role `kms:Decrypt` on that key, with the condition that `kms:ViaService` is `secretsmanager.<secrets_region>.amazonaws.com`. A secret outside the connector's account needs a customer-managed key, because no other account can use the default `aws/secretsmanager` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (`p0_aws_secrets_manager`) for the AWS account, or Google Secret Manager for the Google Cloud project.

Okta reads each AWS account's roles as an IAM user with `iam:ListAccountAliases` and `iam:ListRoles`. Create one in each account that trusts the app, give its access key to the app's provisioning settings, and refresh the app's data after the pool roles exist.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).

## Example Usage

### AWS

```terraform
# Adds the Okta AWS Account Federation app 0oa1b2c3d4e5f6g7h8i9, whose service
# app's private key is in AWS Secrets Manager, in the account of the connector on
# AWS Lambda that the p0_okta_aws_iam_federation_connector example installs. For
# a connector on Cloud Run and a private key in Google Secret Manager, see gcp.tf
# in this directory.
#
# Before you apply this, create the Okta API service app and store its private
# key as the secret okta/acme/p0. In the Okta Admin Console, create an API
# Services app that signs in with a public and private key, generate its key,
# and copy the private key, in JSON format, to the file okta-private-key.json.
# Grant the app these Okta API scopes, and no other:
#
#   okta.apps.manage, okta.schemas.manage, okta.users.read
#
# Then give it a custom admin role with the permissions "Edit users'
# application assignments" and "Manage applications", on a resource set that
# holds all users and the AWS Account Federation app. The example doesn't create
# the secret, which keeps the private key out of Terraform's state.
#
# Creating p0_okta_aws_iam_federation_app has the connector read the private
# key, so store it before then, as the secret's whole value. With the AWS CLI
# signed in to the secret's account, run this where okta-private-key.json is:
#
#   aws secretsmanager create-secret --name okta/acme/p0 \
#     --secret-string file://okta-private-key.json --region us-west-2
#
# If the secret exists, store the key as a new version instead, which is also
# how to rotate it:
#
#   aws secretsmanager put-secret-value --secret-id okta/acme/p0 \
#     --secret-string file://okta-private-key.json --region us-west-2
#
# Then delete okta-private-key.json.
#
# The grant is what P0's Okta (AWS IAM Federation) installer generates for the
# app, with p0_okta_aws_iam_federation_app after it.

locals {
  p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9 = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the service app's private key: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the private
    # key, add a new version to this secret instead.
    private_key_name   = "okta/acme/p0"
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
  alias               = "p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9"
  region              = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_region
  allowed_account_ids = [local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.connector_account_id]
}

data "aws_partition" "p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9" {
  provider = aws.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9
}

# The connector reads the app's private key, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity. Each app adds a policy of its own to that role, and a
# role's inline policies share a limit of 10,240 characters: past a few dozen
# apps, deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9_secret" {
  provider = aws.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9

  name = "ReadOktaPrivateKey-0oa1b2c3d4e5f6g7h8i9"
  role = p0_okta_aws_iam_federation_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadPrivateKey"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_region,
          local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_account_id,
          "secret",
          "${local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.private_key_name}-??????",
        ])
      }],
      local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.kms_key_arn == null ? [] : [{
        Sid      = "DecryptPrivateKey"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_region}.${data.aws_partition.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_account_id == local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.connector_account_id || local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.kms_key_arn != null
      error_message = "The private key is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the private key. IAM
# changes can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_okta_aws_iam_federation_app" "example" {
  id        = "0oa1b2c3d4e5f6g7h8i9"
  connector = p0_okta_aws_iam_federation_connector.example.id
  domain    = "acme.okta.com"

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_account_id
    secrets_region = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.secrets_region
  }
  # The service app's client ID, from its General tab.
  client_id        = "0oa9z8y7x6w5v4u3t2s1"
  private_key_name = local.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9.private_key_name

  depends_on = [aws_iam_role_policy.p0_okta_aws_iam_federation_app_0oa1b2c3d4e5f6g7h8i9_secret]
}

# The app's SAML metadata, which each AWS account's SAML provider trusts (see
# the p0_aws_iam_federation example).
output "okta_saml_metadata_url" {
  value = p0_okta_aws_iam_federation_app.example.saml_metadata_url
}
```

### Google Cloud

```terraform
# The Google Cloud variant of the p0_okta_aws_iam_federation_app example in this
# directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_okta_aws_iam_federation_connector example), and its service app's private
# key is in Google Secret Manager.
#
# Before you apply this, create the Okta API service app, as the example in
# resource.tf says, and store its private key as the secret okta-acme-eu-p0.
# The example doesn't create the secret, which keeps the private key out of
# Terraform's state. Run this where okta-private-key.json is:
#
#   gcloud secrets create okta-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=okta-private-key.json
#
# If the secret exists, store the key as a new version instead, which is also
# how to rotate it:
#
#   gcloud secrets versions add okta-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=okta-private-key.json
#
# Then delete okta-private-key.json.

locals {
  p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0 = {
    # The secret that holds the service app's private key, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the private key, add a new version to this secret instead.
    private_key_project = "my-secrets-project-id"
    private_key_id      = "okta-acme-eu-p0"
  }
}

# The connector reads the private key's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_accessor" {
  project   = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  secret_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_okta_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_viewer" {
  project   = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  secret_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_okta_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the private key.
resource "p0_okta_aws_iam_federation_app" "gcp_example" {
  id        = "0oa2b3c4d5e6f7g8h9i0"
  connector = p0_okta_aws_iam_federation_connector.gcp_example.id
  domain    = "acme-eu.okta.com"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  }
  # The service app's client ID, from its General tab.
  client_id        = "0oa8y7x6w5v4u3t2s1r0"
  private_key_name = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_accessor,
    google_secret_manager_secret_iam_member.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `client_id` (String) The 20-character client ID of the Okta API service app that the connector signs in as
- `connector` (String) The connector that acts on this app: the `id` of a `p0_okta_aws_iam_federation_connector` resource. Requires that connector to be installed.
- `domain` (String) The org's Okta domain, such as `acme.okta.com`, in lowercase. A custom domain can't be used.
- `id` (String) The 20-character ID of the Okta AWS Account Federation app, as its URL in the Okta Admin Console shows it
- `private_key_name` (String) The name of the secret that holds the service app's private key as a JWK, such as `okta/acme/p0` in AWS Secrets Manager or `okta-acme-p0` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like `-AbCdEf`, because Secrets Manager can't find a secret by such a name. To rotate the private key, store the new one as a new version of the same secret, which needs no change here.
- `vault` (Attributes) The secret manager that holds the private key. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `entity_id` (String) The app's SAML entity ID, which P0 reads from Okta when it checks the install
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

- `account_id` (String) The ID of the AWS account that holds the private key (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the private key (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the private key is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
