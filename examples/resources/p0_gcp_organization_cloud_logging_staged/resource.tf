# Staging tells P0 where the organization log source's connector runs. P0
# answers with connector_service_name, connector_service_account and region,
# which the connector's Terraform deploys with. Requires p0_gcp. See the
# p0_gcp_organization_cloud_logging example for the full chain.
resource "p0_gcp_organization_cloud_logging_staged" "example" {
  host_project_id = "my-connector-project"
}
