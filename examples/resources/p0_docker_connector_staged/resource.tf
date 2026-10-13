# Staging tells P0 where the connector will run, and names the identity it must
# run as (connector_identity). Requires p0_aws_iam_write for the account. See
# the p0_docker_connector example for the full chain.
resource "p0_docker_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "aws"
    account_id       = p0_aws_iam_write.example.id
    connector_name   = "p0-docker-prod"
    connector_region = "us-west-2"
  }
}
