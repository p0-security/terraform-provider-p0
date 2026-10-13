# The Google Cloud variant of the p0_docker_organization example in this
# directory: the organization's connector runs on Cloud Run (see gcp.tf in the
# p0_docker_connector example), and its organization access token is in Google
# Secret Manager.
#
# Before you apply this, create the organization access token, as the example
# in resource.tf says, and store it as the secret docker-my-other-org-token. The
# example doesn't create the secret, which keeps the token out of Terraform's
# state. To store it, read the token without showing it:
#
#   read -rs DOCKER_TOKEN
#
# Then create the secret:
#
#   printf %s "$DOCKER_TOKEN" | gcloud secrets create docker-my-other-org-token \
#     --project my-secrets-project-id --data-file=-
#
# If the secret exists, store the token as a new version instead, which is also
# how to rotate it:
#
#   printf %s "$DOCKER_TOKEN" | gcloud secrets versions add docker-my-other-org-token \
#     --project my-secrets-project-id --data-file=-

locals {
  p0_docker_org_my_other_org = {
    # The secret that holds my-other-org's access token, and its project, which
    # needn't be the connector's. A new secret replaces the installation. To
    # rotate the token, add a new version to this secret instead.
    token_secret_project = "my-secrets-project-id"
    token_secret_id      = "docker-my-other-org-token"
  }
}

# The connector reads the access token's value. Each binding adds one member to
# the secret's IAM policy, and replaces nothing. Both are on the secret and not
# on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_docker_org_my_other_org_token_accessor" {
  project   = local.p0_docker_org_my_other_org.token_secret_project
  secret_id = local.p0_docker_org_my_other_org.token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_docker_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_docker_org_my_other_org_token_viewer" {
  project   = local.p0_docker_org_my_other_org.token_secret_project
  secret_id = local.p0_docker_org_my_other_org.token_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_docker_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the organization. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the token.
resource "p0_docker_organization" "gcp_example" {
  id        = "my-other-org"
  connector = p0_docker_connector.gcp_example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_docker_org_my_other_org.token_secret_project
  }
  token_secret_name = local.p0_docker_org_my_other_org.token_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_docker_org_my_other_org_token_accessor,
    google_secret_manager_secret_iam_member.p0_docker_org_my_other_org_token_viewer,
  ]
}
