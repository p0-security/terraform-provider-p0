resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Custom role required for P0 to manage IAM grants in your project.
resource "google_project_iam_custom_role" "example" {
  project     = local.project
  role_id     = p0_gcp_iam_write_staged.example.custom_role.id
  title       = p0_gcp_iam_write_staged.example.custom_role.name
  description = "Integration role for P0 IAM management integration"
  permissions = p0_gcp_iam_write_staged.example.permissions
}

resource "google_project_iam_member" "example_custom_role" {
  project = local.project
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Predefined role required for P0 to grant resource-level access.
resource "google_project_iam_member" "example_predefined_role" {
  project = local.project
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# p0_gcp_iam_write fails validation unless installed after the grants above.
resource "p0_gcp_iam_write" "example" {
  project = local.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}
