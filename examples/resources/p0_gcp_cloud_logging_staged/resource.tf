# Staging tells P0 which project a log source reads and where its connector
# runs. P0 answers with connector_service_name, connector_service_account and
# region, which the connector's Terraform deploys with. Requires p0_gcp. See the
# p0_gcp_cloud_logging example for the full chain.
resource "p0_gcp_cloud_logging_staged" "example" {
  project_id      = "my-project-123"
  host_project_id = "my-connector-project"
}
