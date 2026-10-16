# Adds the 1Password vault Engineering, whose ID is 3fz7kq2xmw4vjn6rbt5hlpd8yc
# and whose service account token is in AWS Secrets Manager, in the account of
# the connector on AWS Lambda that the p0_1password_connector example installs.
# For a connector on Cloud Run and a token in Google Secret Manager, see gcp.tf
# in this directory.
#
# Before you apply this, create a 1Password service account for this vault
# alone, with the permissions Read Items and Share Items on it and no other
# vault, and store its token as the secret 1password/engineering/token. The
# token starts with ops_. The example doesn't create the secret, which keeps
# the token out of Terraform's state.
#
# Creating p0_1password_vault has the connector read the token, so the token
# must be in its secret by then. To store it, with the AWS CLI signed in to the
# secret's account, read the token without showing it:
#
#   read -rs OP_SERVICE_ACCOUNT_TOKEN
#
# Then create the secret:
#
#   printf %s "$OP_SERVICE_ACCOUNT_TOKEN" | aws secretsmanager create-secret \
#     --name 1password/engineering/token \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the token as a new version instead, which is also
# how to rotate it:
#
#   printf %s "$OP_SERVICE_ACCOUNT_TOKEN" | aws secretsmanager put-secret-value \
#     --secret-id 1password/engineering/token \
#     --secret-string file:///dev/stdin --region us-west-2

locals {
  p0_1password_vault_engineering = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the vault's service account token: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the token,
    # add a new version to this secret instead.
    token_secret_name  = "1password/engineering/token"
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
  alias               = "p0_1password_vault_engineering"
  region              = local.p0_1password_vault_engineering.secrets_region
  allowed_account_ids = [local.p0_1password_vault_engineering.connector_account_id]
}

data "aws_partition" "p0_1password_vault_engineering" {
  provider = aws.p0_1password_vault_engineering
}

# The connector reads the vault's service account token, and no other secret.
# The policy goes on the connector's role, which P0 names in connector_identity.
# Each vault adds a policy of its own to that role, and a role's inline
# policies share a limit of 10,240 characters: past a few dozen vaults,
# deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_1password_vault_engineering_token" {
  provider = aws.p0_1password_vault_engineering

  name = "Read1PasswordToken-3fz7kq2xmw4vjn6rbt5hlpd8yc"
  role = p0_1password_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadAccessToken"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_1password_vault_engineering.secrets_region,
          local.p0_1password_vault_engineering.secrets_account_id,
          "secret",
          "${local.p0_1password_vault_engineering.token_secret_name}-??????",
        ])
      }],
      local.p0_1password_vault_engineering.kms_key_arn == null ? [] : [{
        Sid      = "DecryptAccessToken"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_1password_vault_engineering.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_1password_vault_engineering.secrets_region}.${data.aws_partition.p0_1password_vault_engineering.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_1password_vault_engineering.secrets_account_id == local.p0_1password_vault_engineering.connector_account_id || local.p0_1password_vault_engineering.kms_key_arn != null
      error_message = "The service account token's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the vault. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the token. IAM changes
# can take a few seconds to take effect, so if the connector is denied the
# secret, apply again.
resource "p0_1password_vault" "example" {
  id        = "3fz7kq2xmw4vjn6rbt5hlpd8yc"
  connector = p0_1password_connector.example.id

  secret_manager = {
    type           = "aws-sm"
    account_id     = local.p0_1password_vault_engineering.secrets_account_id
    secrets_region = local.p0_1password_vault_engineering.secrets_region
  }
  token_secret_name = local.p0_1password_vault_engineering.token_secret_name
  sign_in_address   = "acme.1password.com"

  # Each share link opens only once, so the person gets one look at the item
  # and should copy what they need. Unlike the fields above, this changes in
  # place, and applies to links made after the change. Off by default.
  one_time_links = true

  depends_on = [aws_iam_role_policy.p0_1password_vault_engineering_token]
}

# The email domains the connector shares items with, as P0 read them back from
# it.
output "onepassword_allowed_domains" {
  value = p0_1password_vault.example.allowed_domains
}
