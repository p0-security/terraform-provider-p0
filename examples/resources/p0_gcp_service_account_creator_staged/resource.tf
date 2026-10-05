# Staging generates the identifiers (connector_service_name,
# connector_service_account, region) needed to deploy P0's service account
# connector in the project. Requires p0_gcp_iam_write and p0_gcp_wif_identity
# on the same project. See the p0_gcp_service_account_creator example for the
# full chain.
resource "p0_gcp_service_account_creator_staged" "example" {
  project_id = "my-project-id"
}
