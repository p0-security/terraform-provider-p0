
resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# Follow instructions for creating Terraform for IAM management in p0_gcp_iam_write documentation
# ...

resource "p0_gcp_iam_write" "example" {
  project = locals.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}

# Follow instructions for creating Terraform for Deploy the P0 security perimeter in p0_gcp_security_perimeter documentation
# ...
resource "p0_gcp_security_perimeter" "example" {
  project = locals.project
  url     = google_project_iam_member.example_security_perimeter.url
}

