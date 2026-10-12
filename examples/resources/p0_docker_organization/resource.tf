# Adds the Docker Hub organization my-docker-org, whose organization access
# token is in AWS Secrets Manager, in the account of the connector on AWS Lambda
# that the p0_docker_connector example installs. For a connector on Cloud Run
# and a token in Google Secret Manager, see gcp.tf in this directory.
#
# Before you apply this, create the organization access token and store it as
# the secret docker/my-docker-org/token. In Docker Home, open the organization,
# go to Identity & auth, then Access tokens, and generate a token with the
# organization scopes Member Edit and Invite Edit, and no repository scopes.
# Organization access tokens need a Docker Team or Business plan. The example
# doesn't create the secret, which keeps the token out of Terraform's state.
#
# Creating p0_docker_organization has the connector read the token, so the
# token must be in its secret by then. To store it, with the AWS CLI signed in
# to the secret's account, read the token without showing it:
#
#   read -rs DOCKER_TOKEN
#
# Then create the secret:
#
#   printf %s "$DOCKER_TOKEN" | aws secretsmanager create-secret \
#     --name docker/my-docker-org/token \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the token as a new version instead, which is also
# how to rotate it:
#
#   printf %s "$DOCKER_TOKEN" | aws secretsmanager put-secret-value \
#     --secret-id docker/my-docker-org/token \
#     --secret-string file:///dev/stdin --region us-west-2
#
# The grant is what P0's Docker Hub installer generates for the organization,
# with p0_docker_organization after it.

locals {
  p0_docker_org_my_docker_org = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds my-docker-org's access token: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the token,
    # add a new version to this secret instead.
    token_secret_name  = "docker/my-docker-org/token"
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
  alias               = "p0_docker_org_my_docker_org"
  region              = local.p0_docker_org_my_docker_org.secrets_region
  allowed_account_ids = [local.p0_docker_org_my_docker_org.connector_account_id]
}

data "aws_partition" "p0_docker_org_my_docker_org" {
  provider = aws.p0_docker_org_my_docker_org
}

# The connector reads my-docker-org's access token, and no other secret. The
# policy goes on the connector's role, which P0 names in connector_identity.
# Each organization adds a policy of its own to that role, and a role's inline
# policies share a limit of 10,240 characters: past a few dozen organizations,
# deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_docker_org_my_docker_org_token" {
  provider = aws.p0_docker_org_my_docker_org

  name = "ReadDockerHubToken-my-docker-org"
  role = p0_docker_connector.example.connector_identity.role_name
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
          local.p0_docker_org_my_docker_org.secrets_region,
          local.p0_docker_org_my_docker_org.secrets_account_id,
          "secret",
          "${local.p0_docker_org_my_docker_org.token_secret_name}-??????",
        ])
      }],
      local.p0_docker_org_my_docker_org.kms_key_arn == null ? [] : [{
        Sid      = "DecryptAccessToken"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_docker_org_my_docker_org.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_docker_org_my_docker_org.secrets_region}.${data.aws_partition.p0_docker_org_my_docker_org.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_docker_org_my_docker_org.secrets_account_id == local.p0_docker_org_my_docker_org.connector_account_id || local.p0_docker_org_my_docker_org.kms_key_arn != null
      error_message = "The access token's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the token. IAM changes
# can take a few seconds to take effect, so if the connector is denied the
# secret, apply again.
resource "p0_docker_organization" "example" {
  id        = "my-docker-org"
  connector = p0_docker_connector.example.id

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_docker_org_my_docker_org.secrets_account_id
    secrets_region = local.p0_docker_org_my_docker_org.secrets_region
  }
  token_secret_name = local.p0_docker_org_my_docker_org.token_secret_name

  depends_on = [aws_iam_role_policy.p0_docker_org_my_docker_org_token]
}

# The email domains the connector acts for, as P0 read them back from it.
output "docker_allowed_domains" {
  value = p0_docker_organization.example.allowed_domains
}
