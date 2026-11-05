# Installs Cloudflare for agentic access, with P0's Cloudflare connector on
# Cloud Run and the account's install token in Google Secret Manager.
# Full chain: p0_gcp -> p0_cloudflare_ai_staged -> Cloud Run connector and
# install token secret -> install token version (added outside Terraform) ->
# p0_cloudflare_ai.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  account = "my-cloudflare-account"
  # P0 reads the install token from the secret with this exact ID.
  install_token_secret_id = "p0_install_cloudflare_${local.account}_install-token"
}

resource "p0_cloudflare_ai_staged" "example" {
  id         = local.account
  account_id = "0123456789abcdef0123456789abcdef"

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
  account_id   = split("@", p0_cloudflare_ai_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run Cloudflare connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_cloudflare_ai_staged.example.secret_manager.connector_service_name
  location            = p0_cloudflare_ai_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-cloudflare-ai-gcloud:sha-ae47c26@sha256:3d09fdd0383b9b9db62b6d9786ab17226382d328b7d7182d61628089ee2132e5"

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
  role_id     = "p0CloudflareConnectorSecretCreate"
  title       = "P0 Cloudflare connector secret creation"
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
  role_id = "p0CloudflareConnectorSecretManage"
  title   = "P0 Cloudflare connector access token management"
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
    title      = "P0 Cloudflare access token secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.this.number}/secrets/p0_access-token_cloudflare_${local.account}_')"
  }
}

# Before p0_cloudflare_ai is created, add the install token as a version of this
# secret. The install token is an account API token with the Account API Tokens
# Edit and Zone Read permission groups, made by a Super Administrator. Create the
# secret first, add the version, then apply the rest:
#
#   terraform apply -target=google_secret_manager_secret.install_token
#   printf '%s' "$CLOUDFLARE_INSTALL_TOKEN" \
#     | gcloud secrets versions add p0_install_cloudflare_my-cloudflare-account_install-token \
#       --project=my-project-id --data-file=-
#   terraform apply
resource "google_secret_manager_secret" "install_token" {
  project   = local.project
  secret_id = local.install_token_secret_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

resource "google_secret_manager_secret_iam_member" "install_token" {
  project   = local.project
  secret_id = google_secret_manager_secret.install_token.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the secret itself, and the management role above covers the access token
# secrets alone, so the install token needs its own grant.
resource "google_secret_manager_secret_iam_member" "install_token_read" {
  project   = local.project
  secret_id = google_secret_manager_secret.install_token.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install. Creating it verifies that the connector can read the
# install token and use it on the account.
resource "p0_cloudflare_ai" "example" {
  id         = p0_cloudflare_ai_staged.example.id
  account_id = p0_cloudflare_ai_staged.example.account_id

  secret_manager = {
    type       = p0_cloudflare_ai_staged.example.secret_manager.type
    project_id = p0_cloudflare_ai_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_cloud_run_v2_service_iam_member.read_connector,
    google_project_iam_member.connector_secret_create,
    google_project_iam_member.connector_secret_manage,
    google_secret_manager_secret_iam_member.install_token,
    google_secret_manager_secret_iam_member.install_token_read,
  ]
}

# P0 needs only Account API Tokens Edit and Zone Read. Remove any other groups
# from the install token.
output "other_groups" {
  value = p0_cloudflare_ai.example.other_groups
}
