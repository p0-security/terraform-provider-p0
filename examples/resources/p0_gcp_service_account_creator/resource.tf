# Installs the GCP Service Account Creator, which lets P0 create a short-lived
# service account for each agentic session in this project.
# Full chain: p0_gcp -> p0_gcp_iam_write -> p0_gcp_wif_identity (same project) ->
# p0_gcp_service_account_creator_staged -> Cloud Run connector ->
# p0_gcp_service_account_creator.
# See the p0_gcp_iam_write and p0_gcp_wif_identity examples for the first steps.

resource "p0_gcp_service_account_creator_staged" "example" {
  project_id = p0_gcp_iam_write.example.project
  depends_on = [p0_gcp_wif_identity.example]
}

locals {
  sa_creator = p0_gcp_service_account_creator_staged.example
}

# APIs the connector requires.
resource "google_project_service" "sa_creator" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.sa_creator.project_id
  service            = each.key
  disable_on_destroy = false
}

# The connector's only authority: creating and deleting service accounts in
# this project, and managing who may impersonate them.
resource "google_project_iam_custom_role" "sa_creator" {
  project     = local.sa_creator.project_id
  role_id     = "p0AgenticIdentityManager"
  title       = "P0 agentic identity manager"
  description = "Lets P0's connector create and delete agentic session service accounts."
  permissions = [
    "iam.serviceAccounts.create",
    "iam.serviceAccounts.delete",
    "iam.serviceAccounts.get",
    "iam.serviceAccounts.getIamPolicy",
    "iam.serviceAccounts.setIamPolicy",
    "resourcemanager.projects.get",
  ]
}

resource "google_service_account" "sa_creator" {
  project      = local.sa_creator.project_id
  account_id   = split("@", local.sa_creator.connector_service_account)[0]
  display_name = "P0 service account connector"
}

resource "google_project_iam_member" "sa_creator" {
  project = local.sa_creator.project_id
  role    = google_project_iam_custom_role.sa_creator.name
  member  = "serviceAccount:${google_service_account.sa_creator.email}"
}

resource "google_cloud_run_v2_service" "sa_creator" {
  project             = local.sa_creator.project_id
  name                = local.sa_creator.connector_service_name
  location            = local.sa_creator.region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. The invoker binding below
  # and the connector's own token check restrict who can call it.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.sa_creator.email

    containers {
      image = "docker.io/p0security/p0-connector-gcp-service-account:latest"

      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.sa_creator]
}

# Lets P0 call the connector.
resource "google_cloud_run_v2_service_iam_member" "sa_creator_invoker" {
  project  = local.sa_creator.project_id
  location = local.sa_creator.region
  name     = google_cloud_run_v2_service.sa_creator.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it verifies that P0 can reach the connector.
resource "p0_gcp_service_account_creator" "example" {
  project_id = local.sa_creator.project_id
  depends_on = [
    google_cloud_run_v2_service_iam_member.sa_creator_invoker,
    google_project_iam_member.sa_creator,
  ]
}
