# Complete the installation once the Cloud Run connector is deployed
resource "p0_gcp_cloudsql" "example" {
  id         = p0_gcp_cloudsql_staged.example.id
  project_id = p0_gcp_cloudsql_staged.example.project_id
  depends_on = [module.p0_cloudsql_connector]
}
