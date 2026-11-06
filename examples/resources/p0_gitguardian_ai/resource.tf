# Installs GitGuardian for agentic access, with P0's GitGuardian connector on
# Cloud Run and the workspace's admin token in Google Secret Manager.
# Full chain: p0_gcp -> p0_gitguardian_ai_staged -> Cloud Run connector and
# admin token secret -> admin token version (added outside Terraform) ->
# p0_gitguardian_ai.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  # The GitGuardian workspace ID.
  workspace = "123456"
  # P0 reads the admin token from the secret with this exact ID.
  admin_token_secret_id = "p0_install_gitguardian_${local.workspace}_admin-token"
}

resource "p0_gitguardian_ai_staged" "example" {
  id     = local.workspace
  region = "us"

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
  account_id   = split("@", p0_gitguardian_ai_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run GitGuardian connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_gitguardian_ai_staged.example.secret_manager.connector_service_name
  location            = p0_gitguardian_ai_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-gitguardian-ai-gcloud:sha-6590b2e@sha256:fb52867e7fb673478d940622f3178127d424dbdbc2166c34058f3dd2609dd0d2"

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

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "read_connector" {
  project  = local.project
  location = google_cloud_run_v2_service.connector.location
  name     = google_cloud_run_v2_service.connector.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

data "google_project" "this" {
  project_id = local.project
}

# The connector keeps each access token in its own secret. GCP checks
# secretmanager.secrets.create on the project, so this binding has no condition.
resource "google_project_iam_custom_role" "connector_secret_create" {
  project     = local.project
  role_id     = "p0GitguardianConnectorSecretCreate"
  title       = "P0 GitGuardian connector secret creation"
  permissions = ["secretmanager.secrets.create"]

  depends_on = [google_project_service.enable_services]
}

resource "google_project_iam_member" "connector_secret_create" {
  project = local.project
  role    = google_project_iam_custom_role.connector_secret_create.name
  member  = "serviceAccount:${google_service_account.connector.email}"
}

resource "google_project_iam_custom_role" "connector_secret_manage" {
  project = local.project
  role_id = "p0GitguardianConnectorSecretManage"
  title   = "P0 GitGuardian connector access token management"
  permissions = [
    "secretmanager.secrets.delete",
    "secretmanager.secrets.get",
    "secretmanager.versions.add",
  ]

  depends_on = [google_project_service.enable_services]
}

resource "google_project_iam_member" "connector_secret_manage" {
  project = local.project
  role    = google_project_iam_custom_role.connector_secret_manage.name
  member  = "serviceAccount:${google_service_account.connector.email}"

  # P0 names the access token secrets with this prefix. IAM conditions use the
  # project number, not the project ID.
  condition {
    title      = "P0 GitGuardian access token secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.this.number}/secrets/p0_access-token_gitguardian_${local.workspace}_')"
  }
}

# After apply, add the workspace's admin token as a version of this secret,
# before creating p0_gitguardian_ai, which checks it:
#
#   printf '%s' "$GITGUARDIAN_ADMIN_TOKEN" \
#     | gcloud secrets versions add p0_install_gitguardian_123456_admin-token \
#       --project=my-project-id --data-file=-
resource "google_secret_manager_secret" "admin_token" {
  project   = local.project
  secret_id = local.admin_token_secret_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

resource "google_secret_manager_secret_iam_member" "admin_token" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_token.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the secret itself, and the management role above covers the access token
# secrets alone, so the admin token needs its own grant.
resource "google_secret_manager_secret_iam_member" "admin_token_read" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_token.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install; creating it has the connector check the admin token.
resource "p0_gitguardian_ai" "example" {
  id     = p0_gitguardian_ai_staged.example.id
  region = p0_gitguardian_ai_staged.example.region

  secret_manager = {
    type       = p0_gitguardian_ai_staged.example.secret_manager.type
    project_id = p0_gitguardian_ai_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_cloud_run_v2_service_iam_member.read_connector,
    google_project_iam_member.connector_secret_create,
    google_project_iam_member.connector_secret_manage,
    google_secret_manager_secret_iam_member.admin_token,
    google_secret_manager_secret_iam_member.admin_token_read,
  ]
}
