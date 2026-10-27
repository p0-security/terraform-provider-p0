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
