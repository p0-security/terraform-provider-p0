# Federated login: users assume AWS roles via Okta SAML. P0 verifies at install:
#   - AWS IAM SAML identity provider built from the Okta "AWS Account Federation" app metadata;
#     P0 checks its entity ID matches the Okta app's (both from the same metadata).
#   - Okta app preconfigured: joinAllRoles = true, SourceIdentity attribute (user.login),
#     webSSOAllowedClient = your login app, boolean "Managed by P0" (managedByP0) app-user schema
#     property; also manually "Enable API Integration" in Provisioning.
#   - Installed P0 Okta integration with okta.apps.manage, okta.groups.manage, okta.schemas.manage,
#     okta.users.read scopes.
#   - Pool of P0 grant roles (P0GrantsRole0..N under /p0-grants/) whose trust allows
#     sts:AssumeRoleWithSAML and sts:SetSourceIdentity from that SAML provider.
# p0-terraform-install automates the app and roles (okta_aws_federation, aws_p0_roles modules).

# Stage the account: computes the role name, trust policy, and inline policy P0 needs to manage IAM.
resource "p0_aws_iam_write_staged" "example_staged" {
  id = "123456789012"
}

resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example_staged.role.name
  assume_role_policy = p0_aws_iam_write_staged.example_staged.role.trust_policy
}

# Standalone policy resource: the aws_iam_role inline_policy block was removed in AWS provider v6.
resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example_staged.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example_staged.role.inline_policy
}

# Finalize only after the role and inline policy exist; install verification assumes the role.
resource "p0_aws_iam_write" "example" {
  depends_on = [aws_iam_role_policy.p0_iam_manager]
  id         = p0_aws_iam_write_staged.example_staged.id
  login = {
    type = "federated"
    provider = {
      type   = "okta"
      app_id = "0oabcdefghijKlmN123"
      # Name of the AWS IAM SAML identity provider (prerequisite above).
      identity_provider = "p0_example_okta"
      method = {
        type = "saml"
        account_count = {
          type = "single"
        }
      }
    }
  }
}
