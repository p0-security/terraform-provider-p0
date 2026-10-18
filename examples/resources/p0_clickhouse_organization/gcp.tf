# The Google Cloud variant of the p0_clickhouse_organization example in this
# directory: the organization's connector runs on Cloud Run (see gcp.tf in the
# p0_clickhouse_connector example), and its API key is in Google Secret
# Manager.
#
# Before you apply this, create the p0-connector role and the API key, as the
# example in resource.tf says, and store the key as the secret
# clickhouse-acme-api-key, as JSON: {"keyId": "...", "keySecret": "..."}. The
# example doesn't create the secret, which keeps the API key out of Terraform's
# state. To store it, read the key ID, then the key secret without showing it:
#
#   read -r CLICKHOUSE_KEY_ID && read -rs CLICKHOUSE_KEY_SECRET
#
# Then create the secret:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | gcloud secrets create clickhouse-acme-api-key \
#     --project my-secrets-project-id --data-file=-
#
# If the secret exists, store the API key as a new version instead:
#
#   printf '{"keyId":"%s","keySecret":"%s"}' "$CLICKHOUSE_KEY_ID" "$CLICKHOUSE_KEY_SECRET" | gcloud secrets versions add clickhouse-acme-api-key \
#     --project my-secrets-project-id --data-file=-
#
# Then clear the API key from your shell:
#
#   unset CLICKHOUSE_KEY_ID CLICKHOUSE_KEY_SECRET
#
# To rotate the key, store the new one the same way, as resource.tf says.

locals {
  p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d = {
    # The secret that holds the organization's API key, and its project, which
    # needn't be the connector's. A new secret replaces the installation. To
    # rotate the API key, add a new version to this secret instead.
    api_key_secret_project = "my-secrets-project-id"
    api_key_secret_id      = "clickhouse-acme-api-key"
  }
}

# The connector reads the API key's value. Each binding adds one member to the
# secret's IAM policy, and replaces nothing. Both are on the secret and not on
# its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_accessor" {
  project   = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  secret_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_clickhouse_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_viewer" {
  project   = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  secret_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_clickhouse_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the API key.
resource "p0_clickhouse_organization" "gcp_example" {
  id        = "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
  connector = p0_clickhouse_connector.gcp_example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_project
  }
  api_key_secret_name = local.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d.api_key_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_accessor,
    google_secret_manager_secret_iam_member.p0_clickhouse_organization_9a8b7c6d_5e4f_4a3b_8c2d_1e0f9a8b7c6d_secret_viewer,
  ]
}
