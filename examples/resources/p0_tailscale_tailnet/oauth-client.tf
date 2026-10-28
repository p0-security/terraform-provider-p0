# The OAuth client variant of the p0_tailscale_tailnet example, for a tailnet
# where a federated identity can't be used: the connector reads a Tailscale OAuth
# client's secret from AWS Secrets Manager, in the account of the connector on
# AWS Lambda that the p0_tailscale_connector example installs.
#
# Before you apply this, create the OAuth client in Tailscale's admin console,
# under Settings > Trust credentials, with the scopes the p0_tailscale_tailnet
# resource lists, and store its secret as the secret tailscale/acme/oauth. The
# example doesn't create the client or the secret, which keeps the client's
# secret out of Terraform's state.
#
# Creating p0_tailscale_tailnet has the connector read the secret, so store it
# before then, as the secret's whole value. To store it, with the AWS CLI signed
# in to the secret's account, read the client's secret without showing it:
#
#   read -rs TAILSCALE_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$TAILSCALE_CLIENT_SECRET" | aws secretsmanager create-secret \
#     --name tailscale/acme/oauth \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the client's secret as a new version instead,
# which is also how to rotate it:
#
#   printf %s "$TAILSCALE_CLIENT_SECRET" | aws secretsmanager put-secret-value \
#     --secret-id tailscale/acme/oauth \
#     --secret-string file:///dev/stdin --region us-west-2

locals {
  p0_tailscale_tailnet_t9012cntrl = {
    tailnet_id = "T9012CNTRL"
    # The OAuth client's ID, from Tailscale's Trust credentials page.
    client_id = "kAbc1CNTRL"
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the OAuth client's secret: its name, and the account
    # and region it's in. P0 has the connector read the secret by this name
    # there. To rotate the client's secret, add a new version to this secret.
    client_secret_name = "tailscale/acme/oauth"
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
  alias               = "p0_tailscale_tailnet_t9012cntrl"
  region              = local.p0_tailscale_tailnet_t9012cntrl.secrets_region
  allowed_account_ids = [local.p0_tailscale_tailnet_t9012cntrl.connector_account_id]
}

data "aws_partition" "p0_tailscale_tailnet_t9012cntrl" {
  provider = aws.p0_tailscale_tailnet_t9012cntrl
}

# The connector reads this tailnet's OAuth client secret, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity.
resource "aws_iam_role_policy" "p0_tailscale_tailnet_t9012cntrl_secret" {
  provider = aws.p0_tailscale_tailnet_t9012cntrl

  name = "ReadTailscaleClientSecret-${local.p0_tailscale_tailnet_t9012cntrl.tailnet_id}"
  role = p0_tailscale_connector.example.connector_identity.role_name
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
          local.p0_tailscale_tailnet_t9012cntrl.secrets_region,
          local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id,
          "secret",
          "${local.p0_tailscale_tailnet_t9012cntrl.client_secret_name}-??????",
        ])
      }],
      local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn == null ? [] : [{
        Sid      = "DecryptClientSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_tailscale_tailnet_t9012cntrl.secrets_region}.${data.aws_partition.p0_tailscale_tailnet_t9012cntrl.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id == local.p0_tailscale_tailnet_t9012cntrl.connector_account_id || local.p0_tailscale_tailnet_t9012cntrl.kms_key_arn != null
      error_message = "The OAuth client's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the secret. IAM changes
# can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_tailscale_tailnet" "oauth_example" {
  id        = local.p0_tailscale_tailnet_t9012cntrl.tailnet_id
  connector = p0_tailscale_connector.example.id

  credential = {
    type      = "oauth-client"
    client_id = local.p0_tailscale_tailnet_t9012cntrl.client_id
    vault = {
      type           = "aws-sm"
      account_id     = local.p0_tailscale_tailnet_t9012cntrl.secrets_account_id
      secrets_region = local.p0_tailscale_tailnet_t9012cntrl.secrets_region
    }
    client_secret_name = local.p0_tailscale_tailnet_t9012cntrl.client_secret_name
  }

  depends_on = [aws_iam_role_policy.p0_tailscale_tailnet_t9012cntrl_secret]
}
