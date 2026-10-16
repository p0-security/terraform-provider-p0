# The Google Cloud Run variant of the p0_1password_connector example in this
# directory.
#
# Full chain: p0_gcp (P0's service account) -> p0_1password_connector_staged ->
# the connector's Cloud Run service -> p0_1password_connector ->
# p0_1password_account.
#
# Requires these APIs to be enabled in project my-project-id:
#   - iam.googleapis.com
#   - run.googleapis.com
#   - secretmanager.googleapis.com
#
# One connector serves every 1Password account that names it in P0. Each
# account's own Terraform lets this connector read that account's
# service account token (see gcp.tf in the p0_1password_account example). This
# configuration grants it no secret.

resource "p0_gcp" "gcp_example" {
  organization_id = "123456789012"
}

locals {
  p0_1password_connector_gcp = {
    # Where P0 invokes the connector. None of them can change once the connector
    # is added.
    project_id       = "my-project-id"
    connector_name   = "p0-1password-prod"
    connector_region = "us-west1"
    # The email domains the connector shares items with: a regular expression
    # that the connector tests against the part of an email after the "@".
    # Nothing anchors it for you, so keep the "^" and "$": without them a
    # pattern for acme.com also matches acme.com.evil.io. Change it to cover
    # everyone who may be sent a share link, such as "^(acme\\.com|acme\\.io)$"
    # for two domains. The connector won't start without a pattern. P0 can't
    # change it, and shows the one deployed as allowed_domains on each
    # p0_1password_account.
    domain_allow_pattern = "^example\\.com$"
    # P0's connector image: the digest of its tag on Docker Hub. This is a
    # placeholder, for an image that doesn't exist. Use the pinned value in the
    # Terraform that P0's installer shows, or the image's published digest.
    image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

# Tells P0 where the connector will run, and names the service account it must
# run as.
resource "p0_1password_connector_staged" "gcp_example" {
  id = "prod-gcp"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_1password_connector_gcp.project_id
    connector_name   = local.p0_1password_connector_gcp.connector_name
    connector_region = local.p0_1password_connector_gcp.connector_region
  }

  depends_on = [p0_gcp.gcp_example]
}

# The connector's service account, which P0 names after the connector and
# expects by exactly that name. Each account's Terraform lets it read that
# account's service account token.
resource "google_service_account" "p0_1password_connector_gcp" {
  project      = local.p0_1password_connector_gcp.project_id
  account_id   = p0_1password_connector_staged.gcp_example.connector_identity.service_account_id
  display_name = "P0 1Password connector prod-gcp"
}

resource "google_cloud_run_v2_service" "p0_1password_connector_gcp" {
  project             = local.p0_1password_connector_gcp.project_id
  name                = local.p0_1password_connector_gcp.connector_name
  location            = local.p0_1password_connector_gcp.connector_region
  deletion_protection = false
  # No VPC: P0 calls the connector from outside Google Cloud, and the connector
  # calls 1Password over the internet. Access is gated by IAM, not origin.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_1password_connector_gcp.email

    containers {
      image = "1password.io/p0security/p0-connector-1password-gcloud@${local.p0_1password_connector_gcp.image_digest}"

      # The connector checks this against the caller's OIDC token, so it is
      # not redundant with the roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.gcp_example.service_account_email
      }

      env {
        name  = "DOMAIN_ALLOW_PATTERN"
        value = local.p0_1password_connector_gcp.domain_allow_pattern
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "p0_1password_connector_gcp_invoker" {
  project  = local.p0_1password_connector_gcp.project_id
  location = local.p0_1password_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_1password_connector_gcp.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# P0 reads the service to get its invocation URL when it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "p0_1password_connector_gcp_viewer" {
  project  = local.p0_1password_connector_gcp.project_id
  location = local.p0_1password_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_1password_connector_gcp.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# Completes the install. Creating it has P0 read the service's URL and check
# that it can invoke the connector, so it waits until P0's service account may
# do both. P0 resolves the URL itself, so it is read back from
# hosting.connector_service_uri rather than set.
resource "p0_1password_connector" "gcp_example" {
  id      = p0_1password_connector_staged.gcp_example.id
  hosting = p0_1password_connector_staged.gcp_example.hosting

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_1password_connector_gcp_invoker,
    google_cloud_run_v2_service_iam_member.p0_1password_connector_gcp_viewer,
  ]
}
