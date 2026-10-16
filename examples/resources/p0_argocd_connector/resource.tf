# Installs P0's Argo CD connector on Google Cloud Run. For AWS Lambda, see aws.tf
# in this directory.
#
# Full chain: p0_gcp (see its example) -> p0_argocd_connector_staged -> the
# connector's service account and Cloud Run service -> roles/run.invoker and
# roles/run.viewer for P0's service account -> p0_argocd_connector.

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned needs Terraform 1.2.
  required_version = ">= 1.2"
}

resource "p0_argocd_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-argocd-prod"
    connector_region = "us-west1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  argocd = p0_argocd_connector_staged.example.hosting
}

resource "google_project_service" "argocd" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.argocd.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when the
# connector was staged.
resource "google_service_account" "argocd" {
  project      = local.argocd.project_id
  account_id   = split("@", local.argocd.connector_service_account)[0]
  display_name = "P0 Argo CD connector"

  depends_on = [google_project_service.argocd]
}

resource "google_cloud_run_v2_service" "argocd" {
  project             = local.argocd.project_id
  name                = local.argocd.connector_name
  location            = local.argocd.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.argocd.email

    containers {
      # P0's connector image, pinned by its tag and digest: the build the P0 app
      # pins. Find a newer tag's digest with
      # `docker buildx imagetools inspect p0security/p0-connector-argocd-gcloud:<tag>`.
      image = "docker.io/p0security/p0-connector-argocd-gcloud:sha-3d6f1f4@sha256:121fdb925d3c002964f18091e0097f466c174a8004d084f61ec6e3aa73e9cf08"

      # The connector accepts calls from this identity alone, and refuses every
      # call while it is unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }

      # A regular expression that an email's whole domain must match for its
      # user to receive access through this connector. It is set here alone,
      # and isn't an attribute of p0_argocd_connector: P0 doesn't set it.
      env {
        name  = "ALLOWED_EMAIL_DOMAIN_PATTERN"
        value = "example[.]com"
      }
    }
  }

  depends_on = [google_project_service.argocd]
}

resource "google_cloud_run_v2_service_iam_member" "argocd_invoker" {
  project  = local.argocd.project_id
  location = google_cloud_run_v2_service.argocd.location
  name     = google_cloud_run_v2_service.argocd.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "argocd_viewer" {
  project  = local.argocd.project_id
  location = google_cloud_run_v2_service.argocd.location
  name     = google_cloud_run_v2_service.argocd.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector and read its service.
resource "p0_argocd_connector" "example" {
  id = p0_argocd_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.argocd.type
    project_id       = local.argocd.project_id
    connector_name   = local.argocd.connector_name
    connector_region = local.argocd.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.argocd_invoker,
    google_cloud_run_v2_service_iam_member.argocd_viewer,
  ]
}
