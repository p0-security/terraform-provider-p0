resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# Role granting P0 read of your org's IAM config and asset inventory.
resource "google_organization_iam_custom_role" "example" {
  org_id      = p0_gcp.example.organization_id
  role_id     = "p0IamAuditor"
  title       = "P0 IAM Auditor"
  description = "Integration role for org-wide P0 IAM assessment integration"
  permissions = concat(
    p0_gcp.example.iam_assessment.permissions.project,
    p0_gcp.example.iam_assessment.permissions.organization
  )
}

resource "google_organization_iam_member" "example" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.example.id
  member = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# p0_gcp_organization_iam_assessment fails validation unless installed after the grant above.
resource "p0_gcp_organization_iam_assessment" "example" {
  depends_on = [google_organization_iam_member.example]
}
