# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_github_ai_staged" "example" {
  id = "my-github-org"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# Or, with the connector on AWS Lambda and its secrets in AWS Secrets Manager.
# Requires AWS Secrets Manager to already be installed in P0 for the account.
resource "p0_github_ai_staged" "aws_example" {
  id = "my-other-github-org"

  secret_manager = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }
}

# See the p0_github_ai example for the next steps (deploying the connector and
# its private key secret) that complete the install.
