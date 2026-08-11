# See the p0_gcp_wif_identity_staged example for the preceding steps
# (staging the identity, and creating the matching Workload Identity Pool
# and provider) that this resource depends on.

resource "p0_gcp_wif_identity_staged" "example" {
  id                = "github-actions"
  project_id        = "my-project-id"
  oidc_provider_url = "https://token.actions.githubusercontent.com"
}

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

# Finalizes the install; depends_on ensures the matching GCP-side
# infrastructure exists first.
resource "p0_gcp_wif_identity" "example" {
  id                = p0_gcp_wif_identity_staged.example.id
  project_id        = p0_gcp_wif_identity_staged.example.project_id
  oidc_provider_url = p0_gcp_wif_identity_staged.example.oidc_provider_url
  depends_on        = [google_iam_workload_identity_pool_provider.p0]
}
