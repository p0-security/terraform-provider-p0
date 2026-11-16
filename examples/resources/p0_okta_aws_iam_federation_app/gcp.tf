# The Google Cloud variant of the p0_okta_aws_iam_federation_app example in this
# directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_okta_aws_iam_federation_connector example), and its service app's private
# key is in Google Secret Manager.
#
# Before you apply this, create the Okta API service app, as the example in
# resource.tf says, and store its private key as the secret okta-acme-eu-p0.
# The example doesn't create the secret, which keeps the private key out of
# Terraform's state. Run this where okta-private-key.json is:
#
#   gcloud secrets create okta-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=okta-private-key.json
#
# If the secret exists, store the key as a new version instead, which is also
# how to rotate it:
#
#   gcloud secrets versions add okta-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=okta-private-key.json
#
# Then delete okta-private-key.json.

locals {
  p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0 = {
    # The secret that holds the service app's private key, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the private key, add a new version to this secret instead.
    private_key_project = "my-secrets-project-id"
    private_key_id      = "okta-acme-eu-p0"
  }
}

# The connector reads the private key's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_accessor" {
  project   = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  secret_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_okta_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_viewer" {
  project   = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  secret_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_okta_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the private key.
resource "p0_okta_aws_iam_federation_app" "gcp_example" {
  id        = "0oa2b3c4d5e6f7g8h9i0"
  connector = p0_okta_aws_iam_federation_connector.gcp_example.id
  domain    = "acme-eu.okta.com"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_project
  }
  # The service app's client ID, from its General tab.
  client_id        = "0oa8y7x6w5v4u3t2s1r0"
  private_key_name = local.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0.private_key_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_accessor,
    google_secret_manager_secret_iam_member.p0_okta_aws_iam_federation_app_0oa2b3c4d5e6f7g8h9i0_viewer,
  ]
}
