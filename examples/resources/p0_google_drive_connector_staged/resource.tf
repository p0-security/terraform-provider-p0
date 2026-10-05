# Staging generates the identifiers (connector_service_name,
# connector_service_account, region) needed to deploy P0's Google Drive
# connector. Requires p0_gcp_service_account_creator on the same project. See
# the p0_google_drive_connector example for the full chain.
resource "p0_google_drive_connector_staged" "example" {
  project_id = p0_gcp_service_account_creator.example.project_id
}
