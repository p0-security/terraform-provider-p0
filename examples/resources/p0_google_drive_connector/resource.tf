# Installs P0's Google Drive connector in the project where P0 creates agentic
# identities. Full chain: p0_gcp_service_account_creator (see its example) ->
# p0_google_drive_connector_staged -> Cloud Run connector ->
# p0_google_drive_connector -> p0_google_drive_shared_drive.

resource "p0_google_drive_connector_staged" "example" {
  project_id = p0_gcp_service_account_creator.example.project_id
}

locals {
  drive = p0_google_drive_connector_staged.example
}

# APIs the connector requires.
resource "google_project_service" "drive" {
  for_each = toset([
    "drive.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.drive.project_id
  service            = each.key
  disable_on_destroy = false
}

# The connector holds no role in the project. Its only authority is the shared
# drives it is made a manager of (see p0_google_drive_shared_drive).
resource "google_service_account" "drive" {
  project      = local.drive.project_id
  account_id   = split("@", local.drive.connector_service_account)[0]
  display_name = "P0 Google Drive connector"
}

resource "google_cloud_run_v2_service" "drive" {
  project             = local.drive.project_id
  name                = local.drive.connector_service_name
  location            = local.drive.region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. The invoker binding below
  # and the connector's own token check restrict who can call it.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.drive.email

    containers {
      image = "docker.io/p0security/p0-connector-google-drive-ai:latest"

      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.drive]
}

# Lets P0 call the connector.
resource "google_cloud_run_v2_service_iam_member" "drive_invoker" {
  project  = local.drive.project_id
  location = local.drive.region
  name     = google_cloud_run_v2_service.drive.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it verifies that the connector is deployed.
resource "p0_google_drive_connector" "example" {
  project_id = local.drive.project_id
  depends_on = [google_cloud_run_v2_service_iam_member.drive_invoker]
}

# Add this address as a Manager on each shared drive P0 grants access in.
output "google_drive_connector_service_account" {
  value = p0_google_drive_connector.example.connector_service_account
}
