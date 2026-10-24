# Staging tells P0 where the connector will run. P0 answers with
# hosting.connector_service_account, the service account that the connector's
# Cloud Run service must run as. Requires p0_gcp. See the p0_jira_ai_connector
# example for the full chain.
resource "p0_jira_ai_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-jira-prod"
    connector_region = "us-central1"
  }
}
