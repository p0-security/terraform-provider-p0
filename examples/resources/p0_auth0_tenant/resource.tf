# Adds the Auth0 tenant acme.us.auth0.com, whose application's client secret is
# in AWS Secrets Manager, in the account of the connector on AWS Lambda that the
# p0_auth0_connector example installs. For a connector on Cloud Run and a
# client secret in Google Secret Manager, see gcp.tf in this directory.
#
# Before you apply this, create the machine-to-machine application and store
# its client secret as the secret auth0/acme/mgmt. In the Auth0 Dashboard, go to
# Applications, create a Machine to Machine application, and authorize it for
# the Auth0 Management API with these scopes:
#
#   read:users, read:roles, read:role_members, update:users,
#   create:role_members, delete:role_members, read:resource_servers,
#   read:user_effective_permissions, read:organizations,
#   read:organization_members, read:organization_member_roles,
#   create:organization_members, delete:organization_members,
#   create:organization_member_roles, delete:organization_member_roles
#
# The application's Settings show its client ID and client secret. The example
# doesn't create the secret, which keeps the client secret out of Terraform's
# state.
#
# Creating p0_auth0_tenant has the connector read the client secret, so store
# it in its secret before then, as the secret's whole value. To store it, with the
# AWS CLI signed in to the secret's account, read the client secret without
# showing it:
#
#   read -rs AUTH0_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$AUTH0_CLIENT_SECRET" | aws secretsmanager create-secret \
#     --name auth0/acme/mgmt \
#     --secret-string file:///dev/stdin --region us-west-2
#
# If the secret exists, store the client secret as a new version instead, which
# is also how to rotate it:
#
#   printf %s "$AUTH0_CLIENT_SECRET" | aws secretsmanager put-secret-value \
#     --secret-id auth0/acme/mgmt \
#     --secret-string file:///dev/stdin --region us-west-2
#
# The grant is what P0's Auth0 installer generates for the tenant, with
# p0_auth0_tenant after it.

locals {
  p0_auth0_tenant_acme_us_auth0_com = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds acme.us.auth0.com's client secret: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the client
    # secret, add a new version to this secret instead.
    client_secret_name = "auth0/acme/mgmt"
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
  alias               = "p0_auth0_tenant_acme_us_auth0_com"
  region              = local.p0_auth0_tenant_acme_us_auth0_com.secrets_region
  allowed_account_ids = [local.p0_auth0_tenant_acme_us_auth0_com.connector_account_id]
}

data "aws_partition" "p0_auth0_tenant_acme_us_auth0_com" {
  provider = aws.p0_auth0_tenant_acme_us_auth0_com
}

# The connector reads acme.us.auth0.com's client secret, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity. Each tenant adds a policy of its own to that role, and a
# role's inline policies share a limit of 10,240 characters: past a few dozen
# tenants, deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_auth0_tenant_acme_us_auth0_com_secret" {
  provider = aws.p0_auth0_tenant_acme_us_auth0_com

  name = "ReadAuth0ClientSecret-acme.us.auth0.com"
  role = p0_auth0_connector.example.connector_identity.role_name
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
          local.p0_auth0_tenant_acme_us_auth0_com.secrets_region,
          local.p0_auth0_tenant_acme_us_auth0_com.secrets_account_id,
          "secret",
          "${local.p0_auth0_tenant_acme_us_auth0_com.client_secret_name}-??????",
        ])
      }],
      local.p0_auth0_tenant_acme_us_auth0_com.kms_key_arn == null ? [] : [{
        Sid      = "DecryptClientSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_auth0_tenant_acme_us_auth0_com.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_auth0_tenant_acme_us_auth0_com.secrets_region}.${data.aws_partition.p0_auth0_tenant_acme_us_auth0_com.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_auth0_tenant_acme_us_auth0_com.secrets_account_id == local.p0_auth0_tenant_acme_us_auth0_com.connector_account_id || local.p0_auth0_tenant_acme_us_auth0_com.kms_key_arn != null
      error_message = "The client secret is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the tenant. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the client secret. IAM
# changes can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_auth0_tenant" "example" {
  id        = "acme.us.auth0.com"
  connector = p0_auth0_connector.example.id

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_auth0_tenant_acme_us_auth0_com.secrets_account_id
    secrets_region = local.p0_auth0_tenant_acme_us_auth0_com.secrets_region
  }
  # The machine-to-machine application's client ID, from its Settings.
  client_id          = "Q7z1pX9b2kWmN4rT6vY8cE0dFgHjLs3A"
  client_secret_name = local.p0_auth0_tenant_acme_us_auth0_com.client_secret_name

  depends_on = [aws_iam_role_policy.p0_auth0_tenant_acme_us_auth0_com_secret]
}

# The email domains the connector acts for, as P0 read them back from it.
output "auth0_allowed_domains" {
  value = p0_auth0_tenant.example.allowed_domains
}
