# The Google Cloud variant of the p0_1password_account example in this
# directory: the account's connector runs on Cloud Run (see gcp.tf in the
# p0_1password_connector example), and its service account token is in Google
# Secret Manager.
#
# Before you apply this, create the account's service account, as the example
# in resource.tf says, and store its token as the secret 1password-globex-token.
# The example doesn't create the secret, which keeps the token out of
# Terraform's state. To store it, read the token without showing it:
#
#   read -rs OP_SERVICE_ACCOUNT_TOKEN
#
# Then create the secret:
#
#   printf %s "$OP_SERVICE_ACCOUNT_TOKEN" | gcloud secrets create 1password-globex-token \
#     --project my-secrets-project-id --data-file=-
#
# If the secret exists, store the token as a new version instead, which is also
# how to rotate it:
#
#   printf %s "$OP_SERVICE_ACCOUNT_TOKEN" | gcloud secrets versions add 1password-globex-token \
#     --project my-secrets-project-id --data-file=-

locals {
  p0_1password_account_globex = {
    # The secret that holds the account's service account token, and its
    # project, which needn't be the connector's. A new secret replaces the
    # installation. To rotate the token, add a new version to this secret
    # instead.
    token_secret_project = "my-secrets-project-id"
    token_secret_id      = "1password-globex-token"
  }
}

# The connector reads the service account token's value. Each binding adds one
# member to the secret's IAM policy, and replaces nothing. Both are on the
# secret and not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_1password_account_globex_token_accessor" {
  project   = local.p0_1password_account_globex.token_secret_project
  secret_id = local.p0_1password_account_globex.token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_1password_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_1password_account_globex_token_viewer" {
  project   = local.p0_1password_account_globex.token_secret_project
  secret_id = local.p0_1password_account_globex.token_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_1password_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the account. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the token.
resource "p0_1password_account" "gcp_example" {
  id        = "globex.1password.eu"
  connector = p0_1password_connector.gcp_example.id

  secret_manager = {
    type       = "gcp-sm"
    project_id = local.p0_1password_account_globex.token_secret_project
  }
  token_secret_name = local.p0_1password_account_globex.token_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_1password_account_globex_token_accessor,
    google_secret_manager_secret_iam_member.p0_1password_account_globex_token_viewer,
  ]
}
