# Requires the p0_gcp integration to already be installed for the same
# organization, and the P0 agent access package set up in the org.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_salesforce_ai_staged" "example" {
  id          = "acme-prod"
  full_domain = "acme.my.salesforce.com"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# Or, with the connector on AWS Lambda and its secrets in AWS Secrets Manager.
# Requires AWS Secrets Manager to already be installed in P0 for the account.
resource "p0_salesforce_ai_staged" "aws_example" {
  id          = "acme-sandbox"
  full_domain = "acme.my.salesforce.com"

  secret_manager = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }
}

# See the p0_salesforce_ai example for the next steps (deploying the connector
# and its consumer secret's secret) that complete the install.
