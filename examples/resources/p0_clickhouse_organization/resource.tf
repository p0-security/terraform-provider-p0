# Adds the ClickHouse Cloud organization 4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f,
# whose API key is in AWS Secrets Manager, in the account of the connector on
# AWS Lambda that the p0_clickhouse_connector example installs. For a connector
# on Cloud Run and an API key in Google Secret Manager, see gcp.tf in this
# directory.
#
# Before you apply this, in the ClickHouse Cloud console for the organization,
# create a custom role named p0-connector with exactly these permissions, and
# no others:
#
#   control-plane:organization:view
#   control-plane:organization:manage
#   control-plane:service:view
#   control-plane:service:view-query-api-endpoints
#   control-plane:service:manage-query-api-endpoints
#   control-plane:organization:view-api-keys
#
# control-plane:organization:manage is admin-equivalent: it lets the key create
# roles and add anyone, itself included, to any role. The connector only ever
# changes or deletes roles whose names start with p0_.
#
# Then create an API key with two roles: p0-connector, and the system role
# Basic Service API Reader, which running P0's Query API endpoint needs. Copy
# its key ID and key secret when the console shows them. The example doesn't
# create the secret, which keeps the API key out of Terraform's state.
#
# Creating p0_clickhouse_organization has the connector read the API key, so
# store it in its secret before then, as JSON: {"keyId": "...", "keySecret":
# "..."}. To store it, with the AWS CLI signed in to the secret's account, read
# the key ID, then the key secret without showing it:
#
#   read -r CLICKHOUSE_KEY_ID && read -rs CLICKHOUSE_KEY_SECRET
#
# Then create the secret:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | aws secretsmanager create-secret \
#     --name clickhouse/acme/api-key \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the API key as a new version instead:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | aws secretsmanager put-secret-value \
#     --secret-id clickhouse/acme/api-key \
#     --secret-string file:///dev/stdin --region us-west-2
#
# Then clear the API key from your shell:
#
#   unset CLICKHOUSE_KEY_ID CLICKHOUSE_KEY_SECRET
#
# To rotate the key, create a new key with the same two roles, store it as a
# new version of the secret, then delete the old key. P0 sets up its Query API
# endpoint on a new service at the first SQL request there.
# Neither needs a change in P0 or in this configuration.
#
# The grant is what P0's ClickHouse Cloud installer generates for the
# organization, with p0_clickhouse_organization after it.

locals {
  p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the organization's API key: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the API
    # key, add a new version to this secret instead.
    api_key_secret_name = "clickhouse/acme/api-key"
    secrets_account_id  = "123456789012"
    secrets_region      = "us-west-2"
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
  alias               = "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f"
  region              = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region
  allowed_account_ids = [local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.connector_account_id]
}

data "aws_partition" "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f" {
  provider = aws.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f
}

# The connector reads the organization's API key, and no other secret. The
# policy goes on the connector's role, which P0 names in connector_identity.
# Each organization adds a policy of its own to that role, and a role's inline
# policies share a limit of 10,240 characters: past a few dozen organizations,
# deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f_secret" {
  provider = aws.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f

  name = "ReadClickHouseApiKey-4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f"
  role = p0_clickhouse_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadApiKey"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region,
          local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id,
          "secret",
          "${local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.api_key_secret_name}-??????",
        ])
      }],
      local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn == null ? [] : [{
        Sid      = "DecryptApiKey"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region}.${data.aws_partition.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id == local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.connector_account_id || local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.kms_key_arn != null
      error_message = "The API key's secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the API key. IAM changes
# can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_clickhouse_organization" "example" {
  id        = "4f9b2c1e-7d3a-4e5b-9c8d-0a1b2c3d4e5f"
  connector = p0_clickhouse_connector.example.id

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_account_id
    secrets_region = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.secrets_region
  }
  api_key_secret_name = local.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f.api_key_secret_name

  depends_on = [aws_iam_role_policy.p0_clickhouse_organization_4f9b2c1e_7d3a_4e5b_9c8d_0a1b2c3d4e5f_secret]
}

# The email domains the connector acts for, as P0 read them back from it.
output "clickhouse_allowed_domains" {
  value = p0_clickhouse_organization.example.allowed_domains
}

# Each service that isn't ready for every request, with why, as of the
# organization's last validation in the P0 app or by Terraform. Empty when
# every service is ready.
output "clickhouse_services_not_ready" {
  value = p0_clickhouse_organization.example.service_status
}
