# Installs the GitHub App, with P0's GitHub connector on Cloud Run and the
# App's private key in Google Secret Manager.
# Full chain: p0_gcp -> p0_github_app_staged -> Cloud Run connector and private
# key secret -> private key version (added outside Terraform) -> p0_github_app.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project      = "my-project-id"
  organization = "my-github-org"
  # P0 reads the private key from the secret with this exact ID.
  private_key_secret_id = "p0-install-github-${local.organization}-private-key-pem"
}

resource "p0_github_app_staged" "example" {
  id = local.organization

  secret_manager = {
    type       = "gcp-sm"
    project_id = local.project
  }

  depends_on = [p0_gcp.example]
}

resource "google_project_service" "enable_services" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
  ])
  project            = local.project
  service            = each.key
  disable_on_destroy = false
}

resource "google_service_account" "connector" {
  project      = local.project
  account_id   = split("@", p0_github_app_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run GitHub connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_github_app_staged.example.secret_manager.connector_service_name
  location            = p0_github_app_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-github-gcloud:sha-95bcdfe@sha256:d010bdcde0e69c5c774949a1bd26c7738003157a6b3449c9c45f88180517c51c"

      # The connector also checks this against the caller's OIDC token.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service_iam_member" "invoke_connector" {
  project  = local.project
  location = google_cloud_run_v2_service.connector.location
  name     = google_cloud_run_v2_service.connector.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Scoped to the project because secretmanager.secrets.create cannot be granted
# on a single secret.
resource "google_project_iam_custom_role" "connector_secrets" {
  project = local.project
  role_id = "p0GithubConnectorSecrets"
  title   = "P0 GitHub connector secret management"
  permissions = [
    "secretmanager.secrets.create",
    "secretmanager.secrets.delete",
    "secretmanager.secrets.get",
    "secretmanager.versions.add",
  ]

  depends_on = [google_project_service.enable_services]
}

resource "google_project_iam_member" "connector_secrets" {
  project = local.project
  role    = google_project_iam_custom_role.connector_secrets.name
  member  = "serviceAccount:${google_service_account.connector.email}"
}

# After apply, generate a private key on the GitHub App's settings page and add
# the downloaded .pem as a version of this secret. The connector cannot mint
# access tokens until that version exists.
resource "google_secret_manager_secret" "private_key" {
  project   = local.project
  secret_id = local.private_key_secret_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

resource "google_secret_manager_secret_iam_member" "private_key" {
  project   = local.project
  secret_id = google_secret_manager_secret.private_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install; creating it verifies that the connector is deployed.
resource "p0_github_app" "example" {
  id     = p0_github_app_staged.example.id
  app_id = "123456"

  secret_manager = {
    type       = p0_github_app_staged.example.secret_manager.type
    project_id = p0_github_app_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_project_iam_member.connector_secrets,
    google_secret_manager_secret_iam_member.private_key,
  ]
}
