# Requires the p0_gcp integration to already be installed for the same project
# (see its own example).

# P0 renders gcloud CLI/Terraform instructions for creating the Workload Identity
# Pool and provider; it does not create GCP-side infrastructure on your behalf.
# `audience` is computed by P0 from `project_id` once the identity is staged.
resource "p0_gcp_wif_identity" "example" {
  id                = "github-actions"
  project_id        = "my-project-id"
  oidc_provider_url = "https://token.actions.githubusercontent.com"
}
