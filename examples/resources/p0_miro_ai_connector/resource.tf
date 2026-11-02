# Installs P0's Miro (AI) connector on Google Cloud Run. For AWS Lambda, see
# aws.tf in this directory.
#
# Full chain: p0_gcp (see its example) -> p0_miro_ai_connector_staged ->
# the connector's service account and Cloud Run service -> roles/run.invoker
# and roles/run.viewer for P0's service account -> p0_miro_ai_connector.

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned needs Terraform 1.2.
  required_version = ">= 1.2"
}

resource "p0_miro_ai_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-miro_ai-prod"
    connector_region = "us-west1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  miro_ai = p0_miro_ai_connector_staged.example.hosting
}

resource "google_project_service" "miro_ai" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.miro_ai.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when the
# connector was staged.
resource "google_service_account" "miro_ai" {
  project      = local.miro_ai.project_id
  account_id   = split("@", local.miro_ai.connector_service_account)[0]
  display_name = "P0 Miro (AI) connector"

  depends_on = [google_project_service.miro_ai]
}

resource "google_cloud_run_v2_service" "miro_ai" {
  project             = local.miro_ai.project_id
  name                = local.miro_ai.connector_name
  location            = local.miro_ai.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. IAM, not the caller's
  # network, controls access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.miro_ai.email

    containers {
      # P0's connector image, pinned by its tag and digest. To update, take
      # both from the latest version of this example in the Terraform registry.
      image = "docker.io/p0security/p0-connector-miro-ai-gcloud:sha-7228840@sha256:302594174dd95d46539e2c000dcaadb4322ff940f7db94272631ca1a8c0348a6"

      # The connector accepts calls from this identity alone, and refuses every
      # call while it is unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }

    }
  }

  depends_on = [google_project_service.miro_ai]
}

resource "google_cloud_run_v2_service_iam_member" "miro_ai_invoker" {
  project  = local.miro_ai.project_id
  location = google_cloud_run_v2_service.miro_ai.location
  name     = google_cloud_run_v2_service.miro_ai.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "miro_ai_viewer" {
  project  = local.miro_ai.project_id
  location = google_cloud_run_v2_service.miro_ai.location
  name     = google_cloud_run_v2_service.miro_ai.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector and read its service.
resource "p0_miro_ai_connector" "example" {
  id = p0_miro_ai_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.miro_ai.type
    project_id       = local.miro_ai.project_id
    connector_name   = local.miro_ai.connector_name
    connector_region = local.miro_ai.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.miro_ai_invoker,
    google_cloud_run_v2_service_iam_member.miro_ai_viewer,
  ]
}
