# The p0_gcp resource is the root Google Cloud install; all p0_gcp_* component
# resources (p0_gcp_iam_write, p0_gcp_iam_assessment, p0_gcp_access_logs, ...)
# require it. Its read-only attributes (service_account_email, access_logs,
# iam_assessment, org_wide_policy) are used to create the Google Cloud
# infrastructure that P0 depends on; see each component resource's example for
# its full install chain.
resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# The org_wide_policy attribute has no dedicated component resource, so its
# read-only metadata is consumed here directly. This custom role grants P0
# read access to your organization-wide IAM policies.
resource "google_organization_iam_custom_role" "org_wide_policy" {
  org_id      = p0_gcp.example.organization_id
  role_id     = p0_gcp.example.org_wide_policy.custom_role.id
  title       = p0_gcp.example.org_wide_policy.custom_role.name
  description = "Role for the P0 org-wide policy-read installation"
  permissions = p0_gcp.example.org_wide_policy.permissions
}

# Bind the org-wide policy-read role to P0's computed service account identity.
# service_account_email is the identity P0 uses to communicate with your
# organization; it is the member you grant in every downstream IAM binding.
resource "google_organization_iam_member" "org_wide_policy" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.org_wide_policy.id
  member = "serviceAccount:${p0_gcp.example.service_account_email}"
}
