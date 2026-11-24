# Adds the PingFederate app acme-aws, whose roles P0's connector writes through
# SCIM, and whose SCIM password is in AWS Secrets Manager, in the account of the connector on AWS Lambda that the
# p0_pingfederate_aws_iam_federation_connector example installs. For a connector on
# Cloud Run and a SCIM password in Google Secret Manager, see gcp.tf in this
# directory.
#
# Before you apply this:
#
# - In Active Directory, add the multi-valued string attribute awsRole on
#   users, which P0 adds each user's AWS roles to, and let the account of
#   PingFederate's Active Directory data store write it.
# - In PingFederate, create an IdP connection with only Inbound Provisioning,
#   on SCIM 2.0, on the Active Directory data store. Add awsRole to its custom
#   SCIM schema as a multi-valued, read-write string, and map it to the Active
#   Directory attribute in Write Users and Read Users. In Read Users, also map
#   emails.work.value to mail. Use HTTP Basic authentication with the user name
#   p0-scim, and copy its password to the file pingfederate-scim-password.txt.
# - Create an SP connection to urn:amazon:webservices. Map awsRole to
#   https://aws.amazon.com/SAML/Attributes/Role, and mail to
#   https://aws.amazon.com/SAML/Attributes/RoleSessionName and
#   https://aws.amazon.com/SAML/Attributes/SourceIdentity, with the URI name
#   format. Use the persistent NameID format for the subject, and select
#   "Always sign the SAML Assertion".
#
# The example doesn't create the secret, which keeps the SCIM password out of
# Terraform's state. Creating p0_pingfederate_aws_iam_federation_app has the
# connector read it, so store it before then, as the secret's whole value.
# With the AWS CLI signed in to the secret's account, run this where
# pingfederate-scim-password.txt is:
#
#   aws secretsmanager create-secret --name pingfederate/acme/p0 \
#     --secret-string file://pingfederate-scim-password.txt --region us-west-2
#
# If the secret exists, store the SCIM password as a new version instead,
# which is also how to rotate it:
#
#   aws secretsmanager put-secret-value --secret-id pingfederate/acme/p0 \
#     --secret-string file://pingfederate-scim-password.txt --region us-west-2
#
# Then delete pingfederate-scim-password.txt.
#
# The grant is what P0's PingFederate (AWS IAM Federation) installer generates for
# the app, with p0_pingfederate_aws_iam_federation_app after it.

locals {
  p0_pingfederate_aws_iam_federation_app_acme_aws = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the SCIM password: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the SCIM
    # password, add a new version to this secret instead.
    scim_password_name = "pingfederate/acme/p0"
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
  alias               = "p0_pingfederate_aws_iam_federation_app_acme_aws"
  region              = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region
  allowed_account_ids = [local.p0_pingfederate_aws_iam_federation_app_acme_aws.connector_account_id]
}

data "aws_partition" "p0_pingfederate_aws_iam_federation_app_acme_aws" {
  provider = aws.p0_pingfederate_aws_iam_federation_app_acme_aws
}

# The connector reads the app's SCIM password, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity. Each app adds a policy of its own to that role, and a
# role's inline policies share a limit of 10,240 characters: past a few dozen
# apps, deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_pingfederate_aws_iam_federation_app_acme_aws_secret" {
  provider = aws.p0_pingfederate_aws_iam_federation_app_acme_aws

  name = "ReadPingFederateScimPassword-acme-aws"
  role = p0_pingfederate_aws_iam_federation_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadScimPassword"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region,
          local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id,
          "secret",
          "${local.p0_pingfederate_aws_iam_federation_app_acme_aws.scim_password_name}-??????",
        ])
      }],
      local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptScimPassword"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region}.${data.aws_partition.p0_pingfederate_aws_iam_federation_app_acme_aws.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id == local.p0_pingfederate_aws_iam_federation_app_acme_aws.connector_account_id || local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn != null
      error_message = "The SCIM password is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the SCIM password. IAM
# changes can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_pingfederate_aws_iam_federation_app" "example" {
  id               = "acme-aws"
  pingfederate_url = "https://sso.example.com:9031"

  scim = {
    connector      = p0_pingfederate_aws_iam_federation_connector.example.id
    role_attribute = "awsRole"
    scim_username  = "p0-scim"

    vault = {
      type           = "aws-sm"
      account_id     = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id
      secrets_region = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region
    }
    scim_password_name = local.p0_pingfederate_aws_iam_federation_app_acme_aws.scim_password_name
  }

  depends_on = [aws_iam_role_policy.p0_pingfederate_aws_iam_federation_app_acme_aws_secret]
}

# The app's SAML metadata, which each AWS account's SAML provider trusts (see
# the p0_aws_iam_federation example).
output "pingfederate_saml_metadata_url" {
  value = p0_pingfederate_aws_iam_federation_app.example.saml_metadata_url
}
