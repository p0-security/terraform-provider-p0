# Staging a GCP CloudSQL installation generates the connector identifiers
# (`connector_service_name`, `connector_service_account`, `region`) needed to
# deploy P0's Cloud Run connector before completing the install.
#
# Prerequisites:
#   - The root `p0_gcp` organization install (below).
#   - The `p0_gcp_iam_write` install on the same project (see the
#     examples/resources/p0_gcp_iam_write example for its grant sub-chain).

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# The VPC and subnetwork the CloudSQL instances live on. The connector will be
# given direct VPC access to this subnetwork.
data "google_compute_network" "example" {
  name    = "my-vpc"
  project = local.project
}

data "google_compute_subnetwork" "example" {
  name    = "my-subnet"
  region  = "us-west1"
  project = local.project
}

# Stage the installation to expose the read-only connector identifiers.
resource "p0_gcp_cloudsql_staged" "example" {
  id         = data.google_compute_network.example.name
  project_id = local.project
  subnetwork = data.google_compute_subnetwork.example.name
  depends_on = [p0_gcp.example]
}

# Use the staged outputs above to deploy the P0 CloudSQL Cloud Run connector,
# grant it the CloudSQL roles, let P0 invoke it, and then install the
# `p0_gcp_cloudsql` resource. See the `p0_gcp_cloudsql` example for the full chain.
