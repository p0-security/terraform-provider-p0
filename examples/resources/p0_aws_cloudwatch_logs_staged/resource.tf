# Staging tells P0 which AWS account and region a log source reads. P0 answers
# with connector_name, invoke_role_name, invoke_trust_policy and
# execution_policy, which the connector's Terraform deploys with. See the
# p0_aws_cloudwatch_logs example for the full chain.
resource "p0_aws_cloudwatch_logs_staged" "example" {
  id         = "123456789012:us-west-2"
  account_id = "123456789012"
  region     = "us-west-2"

  # The trail that delivers to CloudWatch Logs in this account and region. Leave
  # it out if there is none.
  cloudtrail = {
    log_group_name = "aws-cloudtrail-logs"
    coverage       = "organization"
  }
}
