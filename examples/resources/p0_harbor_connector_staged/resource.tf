# Staging tells P0 where the connector will run. For Google Cloud, P0 answers
# with hosting.connector_service_account, the service account that the
# connector's Cloud Run service must run as. Requires p0_gcp, or p0_aws_iam_write
# for a connector on AWS Lambda. See the p0_harbor_connector example for the
# full chain.
resource "p0_harbor_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-west1"
  }
}
