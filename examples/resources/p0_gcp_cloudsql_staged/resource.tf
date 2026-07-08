# Stage the GCP CloudSQL installation for a VPC
resource "p0_gcp_cloudsql_staged" "example" {
  id         = "my-vpc"
  project_id = "my-gcp-project"
}

# Deploy the P0 connector's Cloud Run service using the staged connector
# identifiers, e.g. via the p0-connector/gcp module:
#
# module "p0_cloudsql_connector" {
#   source                  = "..."
#   project_id              = p0_gcp_cloudsql_staged.example.project_id
#   region                  = p0_gcp_cloudsql_staged.example.region
#   connector_service_name  = p0_gcp_cloudsql_staged.example.connector_service_name
#   connector_service_account = p0_gcp_cloudsql_staged.example.connector_service_account
# }
