# The Google Cloud Run variant of the p0_tailscale_connector example in this
# directory.
#
# Full chain: p0_gcp (P0's service account) -> p0_tailscale_connector_staged ->
# the connector's Cloud Run service -> p0_tailscale_connector ->
# p0_tailscale_tailnet.
#
# Enable these APIs in project my-project-id first:
#   - iam.googleapis.com
#   - run.googleapis.com
#   - secretmanager.googleapis.com, for a tailnet with an OAuth client
#
# One connector serves every tailnet that names it in P0. Each tailnet's own
# Terraform trusts this connector, or lets it read that tailnet's OAuth client
# secret (see gcp.tf in the p0_tailscale_tailnet example). This configuration
# grants it no credential.

resource "p0_gcp" "gcp_example" {
  organization_id = "123456789012"
}

locals {
  p0_tailscale_connector_gcp = {
    # Where P0 invokes the connector. P0 fixes these when it adds the connector.
    project_id       = "my-project-id"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-west1"
    # P0's connector image: the digest of its tag on Docker Hub. This pins the
    # first posture-only build, sha-19655ef. For a later one, use the pinned
    # value in the Terraform that P0's installer shows.
    image_digest = "sha256:6f75ba4338bcc45fced7144fc49c10417d38ae02d02a146b18cd34e107590007"
  }
}

# Tells P0 where the connector will run, and names the service account it must
# run as.
resource "p0_tailscale_connector_staged" "gcp_example" {
  id = "prod-gcp"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_tailscale_connector_gcp.project_id
    connector_name   = local.p0_tailscale_connector_gcp.connector_name
    connector_region = local.p0_tailscale_connector_gcp.connector_region
  }

  depends_on = [p0_gcp.gcp_example]
}

# The connector's service account, which P0 names after the connector and
# expects by exactly that name. Each tailnet's federated identity trusts it, or
# the tailnet's Terraform lets it read the OAuth client's secret.
resource "google_service_account" "p0_tailscale_connector_gcp" {
  project      = local.p0_tailscale_connector_gcp.project_id
  account_id   = p0_tailscale_connector_staged.gcp_example.connector_identity.service_account_id
  display_name = "P0 Tailscale connector prod-gcp"
}

resource "google_cloud_run_v2_service" "p0_tailscale_connector_gcp" {
  project             = local.p0_tailscale_connector_gcp.project_id
  name                = local.p0_tailscale_connector_gcp.connector_name
  location            = local.p0_tailscale_connector_gcp.connector_region
  deletion_protection = false
  # No VPC: P0 calls the connector from outside Google Cloud, and the connector
  # calls the Tailscale API over the internet. IAM, not the caller's network,
  # gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_tailscale_connector_gcp.email

    containers {
      image = "docker.io/p0security/p0-connector-tailscale-gcloud@${local.p0_tailscale_connector_gcp.image_digest}"

      # The connector checks the caller's OIDC token against this address, on
      # top of the roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.gcp_example.service_account_email
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "p0_tailscale_connector_gcp_invoker" {
  project  = local.p0_tailscale_connector_gcp.project_id
  location = local.p0_tailscale_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_tailscale_connector_gcp.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# P0 reads the service to get the URL it invokes when it installs. The invoker
# role gives run.routes.invoke only, which doesn't cover that read.
resource "google_cloud_run_v2_service_iam_member" "p0_tailscale_connector_gcp_viewer" {
  project  = local.p0_tailscale_connector_gcp.project_id
  location = local.p0_tailscale_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_tailscale_connector_gcp.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# Completes the install. Creating it has P0 read the service's URL and check
# that it can invoke the connector, so it waits until P0's service account may
# do both. P0 looks the URL up itself, so read it from
# hosting.connector_service_uri rather than setting it.
resource "p0_tailscale_connector" "gcp_example" {
  id      = p0_tailscale_connector_staged.gcp_example.id
  hosting = p0_tailscale_connector_staged.gcp_example.hosting

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_tailscale_connector_gcp_invoker,
    google_cloud_run_v2_service_iam_member.p0_tailscale_connector_gcp_viewer,
  ]
}
