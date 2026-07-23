resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  region  = "us-west1"
}

# Staging exposes the read-only attributes (custom_role, required_permissions,
# project_reader_role, allowed_domains, image_digest) needed to build the Cloud
# Run service, service account, and IAM roles before install. The perimeter needs
# no existing p0_gcp_iam_write; install it before (or alongside) iam_write.
# Feature must be enabled for your org; a 404 here means contact P0 support.
resource "p0_gcp_security_perimeter_staged" "example" {
  project    = local.project
  region     = local.region
  depends_on = [p0_gcp.example]
}

# Build that infra from the staged outputs, then install
# p0_gcp_security_perimeter (see that example for the full chain).
