# Installs a Salesforce org for agentic access, with P0's Salesforce connector
# on Cloud Run and the P0 Provisioner app's consumer secret in Google Secret
# Manager. Set up the P0 agent access package in the org first; its setup.sh
# prints the full domain and both consumer keys.
# Full chain: p0_gcp -> p0_salesforce_ai_staged -> Cloud Run connector and
# consumer secret's secret -> consumer secret version (added outside Terraform)
# -> p0_salesforce_ai.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
  org     = "acme-prod"
  # P0 reads the consumer secret from the secret with this exact ID.
  consumer_secret_secret_id = "p0_install_salesforce_${local.org}_consumer-secret"
}

resource "p0_salesforce_ai_staged" "example" {
  id          = local.org
  full_domain = "acme.my.salesforce.com"

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
  account_id   = split("@", p0_salesforce_ai_staged.example.secret_manager.connector_service_account)[0]
  display_name = "P0 Cloud Run Salesforce connector"

  depends_on = [google_project_service.enable_services]
}

resource "google_cloud_run_v2_service" "connector" {
  project             = local.project
  name                = p0_salesforce_ai_staged.example.secret_manager.connector_service_name
  location            = p0_salesforce_ai_staged.example.secret_manager.connector_region
  deletion_protection = false
  # P0 calls the connector from outside GCP; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/p0security/p0-connector-salesforce-ai-gcloud:sha-UNPUBLISHED@sha256:UNPUBLISHED"

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

# After apply, add the P0 Provisioner app's consumer secret as a version of
# this secret, before you create p0_salesforce_ai:
#
#   printf '%s' "$SF_CONSUMER_SECRET" \
#     | gcloud secrets versions add p0_install_salesforce_acme-prod_consumer-secret \
#       --project=my-project-id --data-file=-
resource "google_secret_manager_secret" "consumer_secret" {
  project   = local.project
  secret_id = local.consumer_secret_secret_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.enable_services]
}

# The connector only reads this secret. It makes and changes no secrets.
resource "google_secret_manager_secret_iam_member" "consumer_secret" {
  project   = local.project
  secret_id = google_secret_manager_secret.consumer_secret.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the secret itself.
resource "google_secret_manager_secret_iam_member" "consumer_secret_read" {
  project   = local.project
  secret_id = google_secret_manager_secret.consumer_secret.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.connector.email}"
}

# Completes the install; creating it verifies that the connector can sign in to
# the org with the consumer secret.
resource "p0_salesforce_ai" "example" {
  id          = p0_salesforce_ai_staged.example.id
  full_domain = p0_salesforce_ai_staged.example.full_domain
  # Consumer keys of the package's P0 Provisioner and P0 Agent apps, as setup.sh
  # prints them.
  provisioner_consumer_key = "3MVG9...provisioner"
  agent_consumer_key       = "3MVG9...agent"

  secret_manager = {
    type       = p0_salesforce_ai_staged.example.secret_manager.type
    project_id = p0_salesforce_ai_staged.example.secret_manager.project_id
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.invoke_connector,
    google_cloud_run_v2_service_iam_member.read_connector,
    google_secret_manager_secret_iam_member.consumer_secret,
    google_secret_manager_secret_iam_member.consumer_secret_read,
  ]
}
