resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# Staging exposes the read-only attributes (custom_role, permissions,
# predefined_role) that you must use to build the Google Cloud IAM
# infrastructure P0 needs before completing the install.
resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Use the staged outputs above to create the custom role, grant it and the
# predefined role to P0's service account, and then install the
# `p0_gcp_iam_write` resource. See the `p0_gcp_iam_write` example for the full
# role-creation chain.
