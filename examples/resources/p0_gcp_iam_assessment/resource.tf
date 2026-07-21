resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

resource "p0_gcp_iam_assessment_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Role granting P0 read of project IAM config and asset inventory.
resource "google_project_iam_custom_role" "example" {
  project     = local.project
  role_id     = p0_gcp_iam_assessment_staged.example.custom_role.id
  title       = p0_gcp_iam_assessment_staged.example.custom_role.name
  description = "Integration role for P0 IAM assessment integration"
  permissions = p0_gcp_iam_assessment_staged.example.permissions
}

resource "google_project_iam_member" "example" {
  project = local.project
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# p0_gcp_iam_assessment fails validation unless installed after the grant above.
resource "p0_gcp_iam_assessment" "example" {
  project    = local.project
  depends_on = [google_project_iam_member.example]
}
