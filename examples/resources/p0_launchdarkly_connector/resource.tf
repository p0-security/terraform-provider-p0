# Installs P0's LaunchDarkly connector on Google Cloud Run. For AWS Lambda, see
# aws.tf in this directory.
#
# Full chain: p0_gcp (see its example) -> p0_launchdarkly_connector_staged ->
# the connector's service account and Cloud Run service -> roles/run.invoker
# and roles/run.viewer for P0's service account -> p0_launchdarkly_connector.

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned needs Terraform 1.2.
  required_version = ">= 1.2"
}

resource "p0_launchdarkly_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-launchdarkly-prod"
    connector_region = "us-west1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  launchdarkly = p0_launchdarkly_connector_staged.example.hosting
}

resource "google_project_service" "launchdarkly" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.launchdarkly.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when the
# connector was staged.
resource "google_service_account" "launchdarkly" {
  project      = local.launchdarkly.project_id
  account_id   = split("@", local.launchdarkly.connector_service_account)[0]
  display_name = "P0 LaunchDarkly connector"

  depends_on = [google_project_service.launchdarkly]
}

resource "google_cloud_run_v2_service" "launchdarkly" {
  project             = local.launchdarkly.project_id
  name                = local.launchdarkly.connector_name
  location            = local.launchdarkly.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. IAM, not the caller's
  # network, controls access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.launchdarkly.email

    containers {
      # P0's connector image, pinned by its tag and digest. To update, take
      # both from the latest version of this example in the Terraform registry.
      image = "docker.io/p0security/p0-connector-launchdarkly-gcloud:sha-cbc48e5@sha256:601f8ccff902cbd8ef6e2abfa4d2422b727a89a3a07a66913933cc6c9a3d8dc5"

      # The connector accepts calls from this identity alone, and refuses every
      # call while it is unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }

      # A regular expression that an email's whole domain must match for its
      # member to receive access through this connector. It is set here alone,
      # and isn't an attribute of p0_launchdarkly_connector: P0 doesn't set it.
      env {
        name  = "DOMAIN_ALLOW_PATTERN"
        value = "example[.]com"
      }
    }
  }

  depends_on = [google_project_service.launchdarkly]
}

resource "google_cloud_run_v2_service_iam_member" "launchdarkly_invoker" {
  project  = local.launchdarkly.project_id
  location = google_cloud_run_v2_service.launchdarkly.location
  name     = google_cloud_run_v2_service.launchdarkly.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "launchdarkly_viewer" {
  project  = local.launchdarkly.project_id
  location = google_cloud_run_v2_service.launchdarkly.location
  name     = google_cloud_run_v2_service.launchdarkly.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector and read its service.
resource "p0_launchdarkly_connector" "example" {
  id = p0_launchdarkly_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.launchdarkly.type
    project_id       = local.launchdarkly.project_id
    connector_name   = local.launchdarkly.connector_name
    connector_region = local.launchdarkly.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.launchdarkly_invoker,
    google_cloud_run_v2_service_iam_member.launchdarkly_viewer,
  ]
}
