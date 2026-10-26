# Requires the p0_gcp integration to already be installed for the same
# organization, and the provisioner and JWT bearer inbound integrations set up
# on the instance.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_servicenow_ai_staged" "example" {
  # The instance name, as in acme.service-now.com
  id = "acme"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# Or, with the connector on AWS Lambda and its secrets in AWS Secrets Manager.
# Requires AWS Secrets Manager to already be installed in P0 for the account.
resource "p0_servicenow_ai_staged" "aws_example" {
  # The instance name, as in acme.service-now.com
  id = "acme-dev"

  secret_manager = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }
}

# See the p0_servicenow_ai example for the next steps (deploying the connector
# and its install secret) that complete the install.
