# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_cloudflare_ai_staged" "example" {
  id         = "my-cloudflare-account"
  account_id = "0123456789abcdef0123456789abcdef"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# Or, with the connector on AWS Lambda and its secrets in AWS Secrets Manager.
# Requires AWS Secrets Manager to already be installed in P0 for the account.
resource "p0_cloudflare_ai_staged" "aws_example" {
  id         = "my-other-cloudflare-account"
  account_id = "fedcba9876543210fedcba9876543210"

  secret_manager = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }
}

# See the p0_cloudflare_ai example for the next steps (deploying the connector
# and storing the install token in its secret) that complete the install.
