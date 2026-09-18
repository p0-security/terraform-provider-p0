# The Google Cloud Run variant of the p0_app example in this directory.
#
# Full chain: p0_gcp (P0's service account) -> the connector's Cloud Run service ->
# roles/run.invoker granted to P0's service account -> p0_app.
#
# Two settings control who may call the connector, and both take the same identity:
# the run.invoker grant decides who may reach the service, and INVOKER_SA_EMAIL is
# the identity the connector itself accepts. The connector refuses to start if
# INVOKER_SA_EMAIL is unset.

locals {
  project    = "my-project-id"
  gcp_region = "us-central1"
}

resource "p0_gcp" "gcp_example" {
  organization_id = "123456789012"
}

resource "google_project_service" "run" {
  project            = local.project
  service            = "run.googleapis.com"
  disable_on_destroy = false
}

# The identity the connector runs as. Grant it whatever access your application
# requires.
resource "google_service_account" "connector" {
  account_id = "p0-custom-app-connector"
  project    = local.project
}

# Build and push the connector image outside Terraform, then point this at it.
resource "google_cloud_run_v2_service" "connector" {
  name     = "p0-custom-app-connector"
  project  = local.project
  location = local.gcp_region
  # P0 calls the connector over the internet, authenticated with a signed identity
  # token, so the service accepts public ingress and relies on IAM to gate it.
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  template {
    service_account = google_service_account.connector.email

    containers {
      image = var.connector_image

      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.gcp_example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.run]
}

variable "connector_image" {
  description = "The connector's container image, e.g. us-central1-docker.pkg.dev/my-project-id/p0/connector:1.0.0"
  type        = string
}

# The only permission P0 needs on the connector.
resource "google_cloud_run_v2_service_iam_member" "invoke_connector" {
  project  = local.project
  location = local.gcp_region
  name     = google_cloud_run_v2_service.connector.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# Completes the install. P0 resolves the service's invocation URL itself, so the
# Cloud Run URL is read back from hosting.connector_service_uri rather than set.
resource "p0_app" "gcp_example" {
  id = "internal-admin-tool"

  hosting = {
    type             = "gcp"
    project_id       = local.project
    connector_name   = google_cloud_run_v2_service.connector.name
    connector_region = local.gcp_region
  }

  depends_on = [google_cloud_run_v2_service_iam_member.invoke_connector]
}
