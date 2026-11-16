# The Google Cloud Run variant of the p0_pingone_aws_iam_federation_connector example in this
# directory.
#
# Full chain: p0_gcp (P0's service account) -> p0_pingone_aws_iam_federation_connector_staged -> the
# connector's Cloud Run service -> p0_pingone_aws_iam_federation_connector -> p0_pingone_aws_iam_federation_app.
#
# Enable these APIs in project my-project-id first:
#   - iam.googleapis.com
#   - run.googleapis.com
#   - secretmanager.googleapis.com
#
# One connector serves every PingOne app that names it in P0. Each
# app's own Terraform lets this connector read that app's
# client secret (see gcp.tf in the p0_pingone_aws_iam_federation_app example). This
# configuration grants it no secret.

resource "p0_gcp" "gcp_example" {
  organization_id = "123456789012"
}

locals {
  p0_pingone_aws_iam_federation_connector_gcp = {
    # Where P0 invokes the connector. P0 fixes these when it adds the connector.
    project_id       = "my-project-id"
    connector_name   = "p0-pingone-prod"
    connector_region = "us-west1"
    # P0's connector image: the digest of its tag on Docker Hub. This is a
    # placeholder, for an image that doesn't exist. Use the pinned value in the
    # Terraform that P0's installer shows, or the image's published digest.
    image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

# Tells P0 where the connector will run, and names the service account it must
# run as.
resource "p0_pingone_aws_iam_federation_connector_staged" "gcp_example" {
  id = "prod-gcp"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_pingone_aws_iam_federation_connector_gcp.project_id
    connector_name   = local.p0_pingone_aws_iam_federation_connector_gcp.connector_name
    connector_region = local.p0_pingone_aws_iam_federation_connector_gcp.connector_region
  }

  depends_on = [p0_gcp.gcp_example]
}

# The connector's service account, which P0 names after the connector and
# expects by exactly that name. Each app's Terraform lets it read that
# app's client secret.
resource "google_service_account" "p0_pingone_aws_iam_federation_connector_gcp" {
  project      = local.p0_pingone_aws_iam_federation_connector_gcp.project_id
  account_id   = p0_pingone_aws_iam_federation_connector_staged.gcp_example.connector_identity.service_account_id
  display_name = "P0 PingOne (AWS IAM Federation) connector prod-gcp"
}

resource "google_cloud_run_v2_service" "p0_pingone_aws_iam_federation_connector_gcp" {
  project             = local.p0_pingone_aws_iam_federation_connector_gcp.project_id
  name                = local.p0_pingone_aws_iam_federation_connector_gcp.connector_name
  location            = local.p0_pingone_aws_iam_federation_connector_gcp.connector_region
  deletion_protection = false
  # No VPC: P0 calls the connector from outside Google Cloud, and the connector
  # calls PingOne over the internet. IAM, not the caller's network, gates
  # access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_pingone_aws_iam_federation_connector_gcp.email

    containers {
      image = "docker.io/p0security/p0-connector-pingone-aws-iam-federation-gcloud@${local.p0_pingone_aws_iam_federation_connector_gcp.image_digest}"

      # The connector checks the caller's OIDC token against this address, on
      # top of the roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.gcp_example.service_account_email
      }

    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "p0_pingone_aws_iam_federation_connector_gcp_invoker" {
  project  = local.p0_pingone_aws_iam_federation_connector_gcp.project_id
  location = local.p0_pingone_aws_iam_federation_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_pingone_aws_iam_federation_connector_gcp.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# P0 reads the service to get the URL it invokes when it installs. The invoker
# role gives run.routes.invoke only, which doesn't cover that read.
resource "google_cloud_run_v2_service_iam_member" "p0_pingone_aws_iam_federation_connector_gcp_viewer" {
  project  = local.p0_pingone_aws_iam_federation_connector_gcp.project_id
  location = local.p0_pingone_aws_iam_federation_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_pingone_aws_iam_federation_connector_gcp.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# Completes the install. Creating it has P0 read the service's URL and check
# that it can invoke the connector, so it waits until P0's service account may
# do both. P0 looks the URL up itself, so read it from
# hosting.connector_service_uri rather than setting it.
resource "p0_pingone_aws_iam_federation_connector" "gcp_example" {
  id      = p0_pingone_aws_iam_federation_connector_staged.gcp_example.id
  hosting = p0_pingone_aws_iam_federation_connector_staged.gcp_example.hosting

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_pingone_aws_iam_federation_connector_gcp_invoker,
    google_cloud_run_v2_service_iam_member.p0_pingone_aws_iam_federation_connector_gcp_viewer,
  ]
}
