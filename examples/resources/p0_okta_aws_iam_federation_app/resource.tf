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
# holds all users and the AWS Account Federation app.
#
# On the AWS Account Federation app's Sign On tab, add an attribute statement
# named https://aws.amazon.com/SAML/Attributes/SourceIdentity, with the name
# format Unspecified and the value user.email. AWS then records who signed in
# on each role session.
#
# On the same tab, under "Single use native to web exchange", add the Okta app
# that your users sign in to P0 with. Without it, the P0 CLI can't get AWS
# credentials through the app.
#
# The example doesn't create the secret, which keeps the private key out of
# Terraform's state.
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
