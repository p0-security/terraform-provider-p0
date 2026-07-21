resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  region  = "us-west1"
}

# Staging exposes the read-only attributes (custom_role, required_permissions,
# project_reader_role, allowed_domains, image_digest) that you must use to build
# the Cloud Run service, service account, and IAM roles P0 needs before
# completing the install. The security perimeter does not require an existing
# `p0_gcp_iam_write` install; install it before (or alongside) `p0_gcp_iam_write`.
# The security perimeter must be enabled for your P0 organization; if this
# resource returns a 404, contact P0 support to enable the feature.
resource "p0_gcp_security_perimeter_staged" "example" {
  project    = local.project
  region     = local.region
  depends_on = [p0_gcp.example]
}

# Use the staged outputs above to deploy the P0 security-perimeter Cloud Run
# service, create the invoker and project-reader custom roles, grant them to
# P0's service account, and then install the `p0_gcp_security_perimeter`
# resource. See the `p0_gcp_security_perimeter` example for the full chain.
