# The Google Cloud variant of the p0_auth0_tenant example in this directory: the
# tenant's connector runs on Cloud Run (see gcp.tf in the p0_auth0_connector
# example), and its application's client secret is in Google Secret Manager.
#
# Before you apply this, create the machine-to-machine application, as the
# example in resource.tf says, and store its client secret as the secret
# auth0-acme-eu-mgmt. The example doesn't create the secret, which keeps the
# client secret out of Terraform's state. To store it, read the client secret
# without showing it:
#
#   read -rs AUTH0_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$AUTH0_CLIENT_SECRET" | gcloud secrets create auth0-acme-eu-mgmt \
#     --project my-secrets-project-id --data-file=-
#
# If the secret exists, store the client secret as a new version instead, which
# is also how to rotate it:
#
#   printf %s "$AUTH0_CLIENT_SECRET" | gcloud secrets versions add auth0-acme-eu-mgmt \
#     --project my-secrets-project-id --data-file=-

locals {
  p0_auth0_tenant_acme_eu_auth0_com = {
    # The secret that holds acme.eu.auth0.com's client secret, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the client secret, add a new version to this secret instead.
    client_secret_project = "my-secrets-project-id"
    client_secret_id      = "auth0-acme-eu-mgmt"
  }
}

# The connector reads the client secret's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_auth0_tenant_acme_eu_auth0_com_accessor" {
  project   = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_project
  secret_id = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_auth0_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_auth0_tenant_acme_eu_auth0_com_viewer" {
  project   = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_project
  secret_id = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_auth0_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the tenant. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the client secret.
resource "p0_auth0_tenant" "gcp_example" {
  id        = "acme.eu.auth0.com"
  connector = p0_auth0_connector.gcp_example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_project
  }
  # The machine-to-machine application's client ID, from its Settings.
  client_id          = "R8a2qY0c3lXnO5sU7wZ9dF1eGhIkMt4B"
  client_secret_name = local.p0_auth0_tenant_acme_eu_auth0_com.client_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_auth0_tenant_acme_eu_auth0_com_accessor,
    google_secret_manager_secret_iam_member.p0_auth0_tenant_acme_eu_auth0_com_viewer,
  ]
}
