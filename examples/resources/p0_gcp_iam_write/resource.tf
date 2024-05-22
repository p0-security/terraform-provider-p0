resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my_project_id"
}

resource "p0_gcp_iam_write_staged" "example" {
  project    = locals.project
  depends_on = [p0_gcp.example]
}

# This custom role is required for P0 to manage IAM grants in your project
resource "google_project_iam_custom_role" "example" {
  project     = locals.project
  role_id     = p0_gcp_iam_write_staged.example.custom_role.id
  title       = p0_gcp_iam_write_staged.example.custom_role.name
  description = "Integration role for P0 IAM management integration"
  permissions = p0_gcp_iam_write_staged.example.permissions
}

resource "google_project_iam_member" "example_custom_role" {
  project = locals.project
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# The predefined role is required for P0 to grant resource-level access in your project
resource "google_project_iam_member" "example_predefined_role" {
  project = locals.project
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# The `p0_gcp_iam_write` resource will fail to validate unless it is installed
# _after_ the P0 service account is granted the above roles
resource "p0_gcp_iam_write" "example" {
  project = locals.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}
