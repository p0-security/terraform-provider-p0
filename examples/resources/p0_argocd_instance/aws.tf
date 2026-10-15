# The AWS variant of the p0_argocd_instance example in this directory: an Argo
# CD instance that P0's Argo CD connector on AWS Lambda reaches, with the
# p0-connector account's API token in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_argocd_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the p0-connector account in Argo CD, as this
# resource's page describes, and store its API token as the secret
# argocd/prod/token, in the JSON {"token": "<api token>"}. To store it, with the
# argocd CLI signed in as an admin and the AWS CLI signed in to the secret's
# account:
#
#   argocd account generate-token --account p0-connector |
#     jq -Rc '{token: .}' |
#     aws secretsmanager create-secret --name argocd/prod/token \
#       --secret-string file:///dev/stdin --region us-east-1

locals {
  argocd_instance_aws = {
    url            = "https://argocd.example.com"
    managing_app   = "infra/argocd"
    secrets_region = "us-east-1"
    secret_name    = "argocd/prod/token"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "argocd" {
  provider = aws.argocd
}

# The connector reads the API token's secret, and no other.
resource "aws_iam_role_policy" "argocd_token_secret" {
  provider = aws.argocd

  name = "p0-argocd-prod-aws-read"
  role = aws_iam_role.argocd.id
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
          local.argocd_instance_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.argocd_instance_aws.secret_name}-??????",
        ])
      }],
      local.argocd_instance_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.argocd_instance_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.argocd_instance_aws.secrets_region}.${data.aws_partition.argocd.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the instance through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_argocd_instance" "aws_example" {
  id           = "prod-aws"
  connector    = p0_argocd_connector.aws_example.id
  url          = local.argocd_instance_aws.url
  managing_app = local.argocd_instance_aws.managing_app

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.argocd_instance_aws.secrets_region
  }
  secret_name = local.argocd_instance_aws.secret_name

  depends_on = [aws_iam_role_policy.argocd_token_secret]
}
