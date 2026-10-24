# Installs a Tableau Cloud site for agentic access, with P0's Tableau connector
# on Cloud Run and the admin connected app's secret in Google Secret Manager.
# Full chain: p0_gcp -> p0_tableau_ai_staged -> Cloud Run connector and admin
# connected app secret -> admin connected app version (added outside
# Terraform) -> p0_tableau_ai.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  site    = "my-tableau-site"
  # P0 reads the admin connected app from the secret with this exact ID.
  admin_app_secret_id = "p0_install_tableau_${local.site}_admin-app"
}

resource "p0_tableau_ai_staged" "example" {
  id          = local.site
  server      = "https://us-east-1.online.tableau.com"
  content_url = "acme"

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
  account_id   = split("@", p0_tableau_ai_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run Tableau connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_tableau_ai_staged.example.secret_manager.connector_service_name
  location            = p0_tableau_ai_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-tableau-ai-gcloud:sha-c4202ae@sha256:2fd614def02f746ac93e0544441f1a8db39132f4418e362d94ebb8d2695614b1"

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

# The connector keeps each session token in its own secret. GCP checks
# secretmanager.secrets.create on the project, so this binding has no condition.
resource "google_project_iam_custom_role" "connector_secret_create" {
  project     = local.project
  role_id     = "p0TableauConnectorSecretCreate"
  title       = "P0 Tableau connector secret creation"
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
  role_id = "p0TableauConnectorSecretManage"
  title   = "P0 Tableau connector access token management"
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

  # P0 names the session token secrets with this prefix. IAM conditions use the
  # project number, not the project ID.
  condition {
    title      = "P0 Tableau access token secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.this.number}/secrets/p0_access-token_tableau_${local.site}_')"
  }
}

# After apply, create a Direct Trust connected app on the site, generate a
# secret, enable the app, and add its client ID, secret ID and secret value as
# a JSON version of this secret. The connector cannot grant access until that
# version exists:
#
#   printf '{"clientId":"%s","secretId":"%s","secret":"%s"}' "$TABLEAU_CLIENT_ID" "$TABLEAU_SECRET_ID" "$TABLEAU_SECRET" \
#     | gcloud secrets versions add p0_install_tableau_my-tableau-site_admin-app \
#       --project=my-project-id --data-file=-
resource "google_secret_manager_secret" "admin_app" {
  project   = local.project
  secret_id = local.admin_app_secret_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

resource "google_secret_manager_secret_iam_member" "admin_app" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_app.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the secret itself, and the management role above covers the session
# token secrets alone, so the admin connected app needs its own grant.
resource "google_secret_manager_secret_iam_member" "admin_app_read" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_app.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install; creating it verifies that the connector is deployed.
resource "p0_tableau_ai" "example" {
  id          = p0_tableau_ai_staged.example.id
  server      = p0_tableau_ai_staged.example.server
  content_url = p0_tableau_ai_staged.example.content_url
  # The site administrator that the admin connected app signs in as
  admin_user = "tableau-admin@example.com"
  # P0 names agent users by plus-addressing this mailbox, which gets Tableau's
  # invitation email for each new agent user.
  agent_mailbox = "tableau-agents@example.com"

  secret_manager = {
    type       = p0_tableau_ai_staged.example.secret_manager.type
    project_id = p0_tableau_ai_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_cloud_run_v2_service_iam_member.read_connector,
    google_project_iam_member.connector_secret_create,
    google_project_iam_member.connector_secret_manage,
    google_secret_manager_secret_iam_member.admin_app,
    google_secret_manager_secret_iam_member.admin_app_read,
  ]
}
