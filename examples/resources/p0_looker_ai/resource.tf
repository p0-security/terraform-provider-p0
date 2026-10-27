# Installs Looker for agentic access. P0's Looker connector runs on Cloud Run,
# and the instance's Admin key is in Google Secret Manager.
#
# Creating p0_looker_ai runs P0's install check, and the connector signs in to
# Looker with the Admin key for it. So the key must be in its secret before
# p0_looker_ai is created. Apply in three steps:
#
#   1. Create the Admin key's secret:
#        terraform apply -target=google_secret_manager_secret.admin_key
#   2. Add the Admin key as a version of that secret, with the gcloud command
#      shown above the secret below.
#   3. Create everything else:
#        terraform apply

terraform {
  required_providers {
    # 0.85.0 added the p0_looker_ai resources.
    p0 = {
      source  = "p0-security/p0"
      version = ">= 0.85.0"
    }
  }
}

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project  = "my-project-id"
  instance = "acme"
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

# Holds the Looker API key of a Looker service account with the Admin role.
# Only a Looker admin can create that account and its key. You add the key
# outside Terraform, so it never lands in the Terraform state. After step 1
# above, add it as a version of this secret, as JSON:
#
#   printf '{"client_id":"%s","client_secret":"%s"}' "$LOOKER_CLIENT_ID" "$LOOKER_CLIENT_SECRET" \
#     | gcloud secrets versions add p0_install_looker_acme_admin-key \
#       --project=my-project-id --data-file=-
resource "google_secret_manager_secret" "admin_key" {
  project   = local.project
  secret_id = "p0_install_looker_${local.instance}_admin-key"

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

resource "p0_looker_ai_staged" "example" {
  id               = local.instance
  instance_url     = "https://acme.looker.app"
  admin_key_secret = google_secret_manager_secret.admin_key.secret_id

  secret_manager = {
    type       = "gcp-sm"
    project_id = local.project
  }

  depends_on = [p0_gcp.example]
}

resource "google_service_account" "connector" {
  project      = local.project
  account_id   = split("@", p0_looker_ai_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run Looker connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_looker_ai_staged.example.secret_manager.connector_service_name
  location            = p0_looker_ai_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP, so IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-looker-ai-gcloud:sha-587fbe5@sha256:e986310f36a13b95b1e6c926359d7af78a58e98c9c3841d03a268a607e1aceaf"

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

# The connector keeps each grant's Looker API key in its own secret. GCP checks
# secretmanager.secrets.create on the project, so this binding has no condition.
resource "google_project_iam_custom_role" "connector_secret_create" {
  project     = local.project
  role_id     = "p0LookerConnectorSecretCreate"
  title       = "P0 Looker connector secret creation"
  permissions = ["secretmanager.secrets.create"]

  depends_on = [google_project_service.enable_services]
}

resource "google_project_iam_member" "connector_secret_create" {
  project = local.project
  role    = google_project_iam_custom_role.connector_secret_create.name
  member  = "serviceAccount:${google_service_account.connector.email}"
}

# The connector writes each grant's key to its secret, and deletes the secret
# when the grant ends. secrets.get makes the role match the one that P0's
# in-app install generates. The condition below limits all three permissions
# to the Looker grant secrets.
resource "google_project_iam_custom_role" "connector_secret_manage" {
  project = local.project
  role_id = "p0LookerConnectorSecretManage"
  title   = "P0 Looker connector grant secret management"
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

  # P0 names the grant secrets with this prefix. IAM conditions use the project
  # number, not the project ID.
  condition {
    title      = "P0 Looker grant secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.this.number}/secrets/p0_access-token_looker_${local.instance}_')"
  }
}

# The connector reads the Admin key on every call. Grant read on this secret
# alone. P0 names the secret in each call, so a call could point the connector
# at any other secret it can read.
resource "google_secret_manager_secret_iam_member" "admin_key" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version's metadata, so the Admin key needs this grant as well.
resource "google_secret_manager_secret_iam_member" "admin_key_read" {
  project   = local.project
  secret_id = google_secret_manager_secret.admin_key.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install. Create it only once the Admin key is in its secret
# (step 2 above): P0's install check signs in to Looker with that key.
resource "p0_looker_ai" "example" {
  id               = p0_looker_ai_staged.example.id
  instance_url     = p0_looker_ai_staged.example.instance_url
  admin_key_secret = p0_looker_ai_staged.example.admin_key_secret
  # Optional. The Looker folder that agents save content to, from its URL
  # (/folders/<ID>). Without it, content that an agent saves goes to Trash when
  # its grant ends. To set it, uncomment this line and enter your folder's ID.
  # content_folder_id = "<ID>"

  secret_manager = {
    type       = p0_looker_ai_staged.example.secret_manager.type
    project_id = p0_looker_ai_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_cloud_run_v2_service_iam_member.read_connector,
    google_project_iam_member.connector_secret_create,
    google_project_iam_member.connector_secret_manage,
    google_secret_manager_secret_iam_member.admin_key,
    google_secret_manager_secret_iam_member.admin_key_read,
  ]
}
