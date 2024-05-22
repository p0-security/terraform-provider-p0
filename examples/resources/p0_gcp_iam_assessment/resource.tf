resource "p0_gcp" "example" {
  organization_id = "my_gcp_organization_id"
}

locals {
  project = "my_project_id"
}

resource "p0_gcp_iam_assessment_staged" "example" {
  project    = locals.project
  depends_on = [p0_gcp.example]
}

# This role grants P0 access to analyze your project's IAM configuration and asset inventory
resource "google_project_iam_custom_role" "example" {
  project     = locals.project
  role_id     = p0_gcp_iam_assessment_staged.example.custom_role.id
  title       = p0_gcp_iam_assessment_staged.example.custom_role.name
  description = "Integration role for P0 IAM assessment integration"
  permissions = p0_gcp_iam_assessment_staged.example.permissions
}

resource "google_project_iam_member" "example" {
  project = locals.project
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.gcp.service_account_email}"
}

# The `p0_gcp_iam_write` resource will fail to validate unless it is installed
# _after_ the P0 service account is granted the above role
resource "p0_gcp_iam_assessment" "example" {
  project    = locals.project
  depends_on = [google_project_iam_member.example]
}
