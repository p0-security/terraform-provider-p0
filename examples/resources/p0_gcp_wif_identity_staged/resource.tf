# Requires the p0_gcp integration to already be installed for the same project.
resource "p0_gcp_wif_identity_staged" "example" {
  id                = "github-actions"
  project_id        = "my-project-id"
  oidc_provider_url = "https://token.actions.githubusercontent.com"
}

# `audience` is P0's expected Workload Identity Pool provider resource name:
# //iam.googleapis.com/projects/{number}/locations/global/workloadIdentityPools/{pool}/providers/{provider}
locals {
  wif_pool_id     = regex("workloadIdentityPools/([^/]+)/providers", p0_gcp_wif_identity_staged.example.audience)
  wif_provider_id = regex("providers/([^/]+)$", p0_gcp_wif_identity_staged.example.audience)
}

resource "google_iam_workload_identity_pool" "p0" {
  project                   = p0_gcp_wif_identity_staged.example.project_id
  workload_identity_pool_id = local.wif_pool_id
}

resource "google_iam_workload_identity_pool_provider" "p0" {
  project                            = p0_gcp_wif_identity_staged.example.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.p0.workload_identity_pool_id
  workload_identity_pool_provider_id = local.wif_provider_id

  attribute_mapping = {
    "google.subject" = "assertion.sub"
  }

  oidc {
    issuer_uri = p0_gcp_wif_identity_staged.example.oidc_provider_url
  }
}
