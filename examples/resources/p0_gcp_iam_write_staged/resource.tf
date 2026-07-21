resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# Staging exposes custom_role, permissions, and predefined_role — build the IAM
# infrastructure P0 needs from these before completing the install.
resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Grant the custom and predefined roles to P0's service account, then install
# p0_gcp_iam_write (see that example for the full role-creation chain).
