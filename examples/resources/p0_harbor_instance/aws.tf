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
