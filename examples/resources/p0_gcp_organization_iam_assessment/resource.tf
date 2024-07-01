resource "p0_gcp" "example" {
  organization_id = "my_gcp_organization_id"
}

# This role grants P0 access to analyze your organization's IAM configuration and asset inventory
resource "google_organization_iam_custom_role" "example" {
  org_id      = p0_gcp.organization_id
  role_id     = "p0IamAssessor"
  title       = "P0 IAM assessor"
  description = "Integration role for org-wide P0 IAM assessment integration"
  permissions = concat(
    p0_gcp.example.iam_assessment.permissions.project,
    p0_gcp.example.iam_assessment.permissions.organization
  )
}

resource "google_organization_iam_member" "example" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.example.id
  member = "serviceAccount:${p0_gcp.gcp.service_account_email}"
}

# The `p0_gcp_organization_iam_assessment` resource will fail to validate unless it is installed
# _after_ the P0 service account is granted the above role
resource "p0_gcp_organization_iam_assessment" "example" {
  depends_on = [google_organization_iam_member.example]
}
