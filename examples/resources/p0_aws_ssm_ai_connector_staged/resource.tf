# Requires P0's AWS IAM management (see p0_aws_iam_write) and AWS OIDC (see
# p0_aws_oidc_identity) for the connector's AWS account, and P0's AWS Secrets
# Manager integration for the same account (see p0_aws_secrets_manager). P0
# invokes the connector as AWS IAM management's role, and the connector keeps
# each agent's SSH key in that account's AWS Secrets Manager.

resource "p0_aws_ssm_ai_connector_staged" "example" {
  id     = "123456789012"
  region = "us-east-1"
  vault = {
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-east-1"
  }

  # Destroying the connector needs AWS IAM management and AWS OIDC. If this
  # configuration installs them, depend on them, so that Terraform destroys
  # the connector first.
  # depends_on = [p0_aws_iam_write.example, p0_aws_oidc_identity.example]
}

# See the p0_aws_ssm_ai_connector example for the next steps (deploying the
# connector's Lambda and letting P0 invoke it) that complete the install.
