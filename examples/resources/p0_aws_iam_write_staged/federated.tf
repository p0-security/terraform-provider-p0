# Federated login lets users assume AWS roles through an external identity
# provider (Okta) using SAML. Before applying this example, the following must
# already exist -- P0 verifies them at install time:
#
#   1. An AWS IAM SAML identity provider (referenced below as `identity_provider`)
#      whose metadata document comes from the Okta "AWS Account Federation" app.
#      P0 checks that the SAML provider's metadata entity ID matches the Okta
#      app's entity ID, so both must be created from the same metadata.
#   2. The Okta "AWS Account Federation" SAML app, preconfigured with
#      joinAllRoles = true, the SourceIdentity SAML attribute (user.login), the
#      webSSOAllowedClient set to your login app, and a boolean "Managed by P0"
#      (managedByP0) app-user schema property. (You must also manually
#      "Enable API Integration" in the app's Provisioning tab.)
#   3. An installed P0 Okta integration with the okta.apps.manage,
#      okta.groups.manage, okta.schemas.manage, and okta.users.read scopes.
#   4. A pool of P0 grant roles (e.g. P0GrantsRole0..N under path /p0-grants/)
#      whose trust policy allows sts:AssumeRoleWithSAML and sts:SetSourceIdentity
#      from that same SAML identity provider.
#
# The p0-terraform-install repo automates steps 2 and 4 with its
# `okta_aws_federation` and `aws_p0_roles` modules; use those as the canonical
# setup.

# Stage the AWS account. This computes the role name, trust policy, and inline
# policy that P0 requires to manage IAM in the account.
resource "p0_aws_iam_write_staged" "example_staged" {
  id = "123456789012"
}

# The IAM role that P0 assumes to manage IAM grants in your account. Its name and
# trust policy come from the staged resource's computed outputs.
resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example_staged.role.name
  assume_role_policy = p0_aws_iam_write_staged.example_staged.role.trust_policy
}

# The inline policy that grants the role its IAM-management permissions. Attached
# as a standalone resource (the aws_iam_role inline_policy block was removed in
# AWS provider v6).
resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example_staged.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example_staged.role.inline_policy
}

# Finalize the federated install only after P0's role and its inline policy
# exist -- install verification assumes and inspects the role, so this
# depends_on is required.
resource "p0_aws_iam_write" "example" {
  depends_on = [aws_iam_role_policy.p0_iam_manager]
  id         = p0_aws_iam_write_staged.example_staged.id
  login = {
    type = "federated"
    provider = {
      type   = "okta"
      app_id = "0oabcdefghijKlmN123"
      # Name of the AWS IAM SAML identity provider (prerequisite #1 above).
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
