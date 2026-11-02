# The AWS variant of the p0_miro_ai_team example in this directory: a
# Miro team that P0's Miro (AI) connector on AWS Lambda reaches, with the
# connector user's access token secret in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_miro_ai_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the connector user's access token, as this
# resource's page describes, and store it, and nothing else, as the secret
# miro/design/token. To store it, with the token alone in token.txt
# and the AWS CLI signed in to the secret's account:
#
#   aws secretsmanager create-secret --name miro/design/token \
#     --secret-string file://token.txt --region us-east-1

locals {
  miro_ai_team_aws = {
    secrets_region = "us-east-1"
    secret_name    = "miro/design/token"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "miro_ai" {
  provider = aws.miro_ai
}

# The connector reads this team's access token secret, and no other. Each
# team on the connector adds a policy of its own.
resource "aws_iam_role_policy" "miro_ai_team" {
  provider = aws.miro_ai

  name = "p0-miro-ai-design-aws-read"
  role = aws_iam_role.miro_ai.id
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
          local.miro_ai_team_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.miro_ai_team_aws.secret_name}-??????",
        ])
      }],
      local.miro_ai_team_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.miro_ai_team_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.miro_ai_team_aws.secrets_region}.${data.aws_partition.miro_ai.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the team through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_miro_ai_team" "aws_example" {
  # Requests name the team by this, as in board design-aws <board-id> viewer.
  id        = "design-aws"
  connector = p0_miro_ai_connector.aws_example.id

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.miro_ai_team_aws.secrets_region
  }
  token_secret_name = local.miro_ai_team_aws.secret_name

  depends_on = [aws_iam_role_policy.miro_ai_team]
}
