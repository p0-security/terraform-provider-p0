# Staging generates the connector identifiers (connector_service_name,
# connector_service_account, region) needed to deploy P0's Cloud Run connector
# before completing the install. Requires the root p0_gcp install plus
# p0_gcp_iam_write on the same project (grant sub-chain: see its example).

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# VPC/subnet the instances live on; the connector gets direct VPC access here.
data "google_compute_network" "example" {
  name    = "my-vpc"
  project = local.project
}

data "google_compute_subnetwork" "example" {
  name    = "my-subnet"
  region  = "us-west1"
  project = local.project
}

resource "p0_gcp_cloudsql_staged" "example" {
  id         = data.google_compute_network.example.name
  project_id = local.project
  subnetwork = data.google_compute_subnetwork.example.name
  depends_on = [p0_gcp.example]
}

# Use the staged outputs to deploy the connector, grant it the CloudSQL roles,
# let P0 invoke it, then install p0_gcp_cloudsql - see that example for the full chain.
