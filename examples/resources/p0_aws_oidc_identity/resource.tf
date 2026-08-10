# Requires the p0_aws_iam_write integration to already be installed for the same account
# (see its own example for the full staged-role setup).

# P0 renders CLI/Terraform instructions (from oidc_provider_url + audience) for
# creating the AWS IAM identity provider and role that federate this identity;
# it does not create AWS-side infrastructure on your behalf.
resource "p0_aws_oidc_identity" "example" {
  id                = "github-actions"
  account_id        = "123456789012"
  oidc_provider_url = "https://token.actions.githubusercontent.com"
  audience          = "https://github.com/my-org"
}
