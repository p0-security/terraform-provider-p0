# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_gitguardian_ai_staged" "example" {
  # The GitGuardian workspace ID.
  id     = "123456"
  region = "us"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# Or, with the connector on AWS Lambda and its secrets in AWS Secrets Manager.
# Requires AWS Secrets Manager to already be installed in P0 for the account.
resource "p0_gitguardian_ai_staged" "aws_example" {
  id     = "654321"
  region = "eu"

  secret_manager = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }
}

# See the p0_gitguardian_ai example for the next steps (deploying the connector
# and its admin token secret) that complete the install.
