# Adds the Jira Cloud site acme, and lets its connector (see gcp.tf in the
# p0_jira_ai_connector example) use the site's secrets in its vault project,
# and no others.
#
# The grants are what P0's Jira installer generates for the site. They need
# Terraform's google provider at 6.0 or later.
#
# Before you apply this, create the admin service account's OAuth client in
# Atlassian, and store its secret as the secret jira-acme-admin-client-secret.
# The example doesn't create the secret, which keeps the client's secret out of
# Terraform's state. To store it, read it without showing it:
#
#   read -rs JIRA_ADMIN_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$JIRA_ADMIN_CLIENT_SECRET" | gcloud secrets create jira-acme-admin-client-secret \
#     --project my-secrets-project-id --data-file=-
#
# To rotate it, add a new version to the same secret, which needs no change
# here.

locals {
  p0_jira_ai_site_acme = {
    # The site's ID, which the secret prefixes and custom role IDs below carry.
    id = "acme"
    # The project that holds the site's secrets: one of your Google Secret
    # Manager installations in P0.
    vault_project_id = "my-secrets-project-id"
    admin_secret     = "jira-acme-admin-client-secret"
    # The service account the site's connector runs as.
    connector_service_account = p0_jira_ai_connector.example.hosting.connector_service_account
  }
}

locals {
  # Each agent account's client secret starts with this, and each request's
  # token secret with the other. A site's ID holds no underscore, so the
  # underscore after it ends the prefix: no other site's secrets match.
  p0_jira_ai_site_acme_pool_secret_prefix  = "p0_install_jira-ai_${local.p0_jira_ai_site_acme.id}_"
  p0_jira_ai_site_acme_token_secret_prefix = "p0_access-token_jira-ai_${local.p0_jira_ai_site_acme.id}_"
  # Custom role IDs take no hyphen.
  p0_jira_ai_site_acme_role_id = replace(local.p0_jira_ai_site_acme.id, "-", "_")
}

# A condition matches on the resource name, which carries the project number
# and not the project ID.
data "google_project" "p0_jira_ai_site_acme" {
  project_id = local.p0_jira_ai_site_acme.vault_project_id
}

# The connector reads the admin client's secret. The accessor role reads a
# value. The viewer role reads the version's metadata, which the connector also
# reads, and can change nothing.
resource "google_secret_manager_secret_iam_member" "p0_jira_ai_site_acme_admin_accessor" {
  project   = local.p0_jira_ai_site_acme.vault_project_id
  secret_id = local.p0_jira_ai_site_acme.admin_secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

resource "google_secret_manager_secret_iam_member" "p0_jira_ai_site_acme_admin_viewer" {
  project   = local.p0_jira_ai_site_acme.vault_project_id
  secret_id = local.p0_jira_ai_site_acme.admin_secret
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

# The connector reads each agent account's client secret on this site.
resource "google_project_iam_member" "p0_jira_ai_site_acme_pool_accessor" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme agent account secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_pool_secret_prefix}')"
  }
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_pool_viewer" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = "roles/secretmanager.viewer"
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme agent account secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_pool_secret_prefix}')"
  }
}

# Google checks secretmanager.secrets.create on the project, so this binding
# cannot carry the condition that the management binding below does.
resource "google_project_iam_custom_role" "p0_jira_ai_site_acme_token_create" {
  project     = local.p0_jira_ai_site_acme.vault_project_id
  role_id     = "p0JiraAi_${local.p0_jira_ai_site_acme_role_id}_create"
  title       = "P0 Jira site acme token secret creation"
  permissions = ["secretmanager.secrets.create"]
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_token_create" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = google_project_iam_custom_role.p0_jira_ai_site_acme_token_create.name
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

resource "google_project_iam_custom_role" "p0_jira_ai_site_acme_token_manage" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role_id = "p0JiraAi_${local.p0_jira_ai_site_acme_role_id}_manage"
  title   = "P0 Jira site acme token secret management"
  permissions = [
    "secretmanager.secrets.delete",
    "secretmanager.secrets.get",
    "secretmanager.versions.add",
  ]
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_token_manage" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = google_project_iam_custom_role.p0_jira_ai_site_acme_token_manage.name
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme token secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_token_secret_prefix}')"
  }
}

# Adds the site. P0 looks up its cloud ID from Atlassian. The first agent
# account's install check uses the admin client, so the grants come first.
resource "p0_jira_ai_site" "acme" {
  id        = local.p0_jira_ai_site_acme.id
  connector = p0_jira_ai_connector.example.id
  site_url  = "https://acme.atlassian.net"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_jira_ai_site_acme.vault_project_id
  }

  admin_client_id   = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
  admin_secret_name = local.p0_jira_ai_site_acme.admin_secret
  # The project roles P0 adds an agent account to, as your site names them.
  read_role  = "P0 agent read"
  write_role = "P0 agent write"

  depends_on = [
    google_secret_manager_secret_iam_member.p0_jira_ai_site_acme_admin_accessor,
    google_secret_manager_secret_iam_member.p0_jira_ai_site_acme_admin_viewer,
    google_project_iam_member.p0_jira_ai_site_acme_pool_accessor,
    google_project_iam_member.p0_jira_ai_site_acme_pool_viewer,
    google_project_iam_member.p0_jira_ai_site_acme_token_create,
    google_project_iam_member.p0_jira_ai_site_acme_token_manage,
  ]
}
