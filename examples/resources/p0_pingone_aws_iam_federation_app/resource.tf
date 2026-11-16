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
