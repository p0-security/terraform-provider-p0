# Requires P0's AWS integration to be installed for the connector's AWS account
# (see p0_aws_iam_write). P0 invokes the connector as that installation's role.

resource "p0_databricks_connector_staged" "example" {
  id             = "123456789012"
  region         = "us-east-1"
  domain_pattern = "example\\.com"
}

# See the p0_databricks_connector example for the next steps (deploying the
# connector's Lambda and letting P0 invoke it) that complete the install.
