# Root Google Cloud install; every p0_gcp_* component resource requires it. Its
# read-only attributes feed the infrastructure P0 depends on (see each component
# example for its full install chain).
resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# org_wide_policy has no component resource; its metadata is consumed here. This
# role grants P0 read access to your org-wide IAM policies.
resource "google_organization_iam_custom_role" "org_wide_policy" {
  org_id      = p0_gcp.example.organization_id
  role_id     = p0_gcp.example.org_wide_policy.custom_role.id
  title       = p0_gcp.example.org_wide_policy.custom_role.name
  description = "Role for the P0 org-wide policy-read installation"
  permissions = p0_gcp.example.org_wide_policy.permissions
}

# service_account_email is P0's identity: the member you grant in every downstream IAM binding.
resource "google_organization_iam_member" "org_wide_policy" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.org_wide_policy.id
  member = "serviceAccount:${p0_gcp.example.service_account_email}"
}
