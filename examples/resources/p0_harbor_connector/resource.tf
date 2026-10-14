# Installs P0's Harbor connector on Google Cloud Run. For AWS Lambda, see aws.tf
# in this directory.
#
# Full chain: p0_gcp (see its example) -> p0_harbor_connector_staged -> the
# connector's service account and Cloud Run service -> roles/run.invoker and
# roles/run.viewer for P0's service account -> p0_harbor_connector.

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned needs Terraform 1.2.
  required_version = ">= 1.2"
}

resource "p0_harbor_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-west1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  harbor = p0_harbor_connector_staged.example.hosting
}

resource "google_project_service" "harbor" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.harbor.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when the
# connector was staged.
resource "google_service_account" "harbor" {
  project      = local.harbor.project_id
  account_id   = split("@", local.harbor.connector_service_account)[0]
  display_name = "P0 Harbor connector"

  depends_on = [google_project_service.harbor]
}

resource "google_cloud_run_v2_service" "harbor" {
  project             = local.harbor.project_id
  name                = local.harbor.connector_name
  location            = local.harbor.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.harbor.email

    containers {
      # P0's connector image, pinned by its tag and digest. Use the tag and
      # digest of the release you want to run.
      image = "docker.io/p0security/p0-connector-harbor-gcloud:sha-1234567@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

      # The connector accepts calls from this identity alone, and refuses every
      # call while it is unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }

      # A regular expression that an email's whole domain must match for its
      # user to receive access through this connector. It is set here alone,
      # and isn't an attribute of p0_harbor_connector: P0 doesn't set it.
      env {
        name  = "ALLOWED_EMAIL_DOMAIN_PATTERN"
        value = "example[.]com"
      }
    }
  }

  depends_on = [google_project_service.harbor]
}

resource "google_cloud_run_v2_service_iam_member" "harbor_invoker" {
  project  = local.harbor.project_id
  location = google_cloud_run_v2_service.harbor.location
  name     = google_cloud_run_v2_service.harbor.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "harbor_viewer" {
  project  = local.harbor.project_id
  location = google_cloud_run_v2_service.harbor.location
  name     = google_cloud_run_v2_service.harbor.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector and read its service.
resource "p0_harbor_connector" "example" {
  id = p0_harbor_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.harbor.type
    project_id       = local.harbor.project_id
    connector_name   = local.harbor.connector_name
    connector_region = local.harbor.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.harbor_invoker,
    google_cloud_run_v2_service_iam_member.harbor_viewer,
  ]
}
