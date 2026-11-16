# The Google Cloud variant of the p0_pingone_aws_iam_federation_app example in
# this directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_pingone_aws_iam_federation_connector example), and its worker app's client
# secret is in Google Secret Manager.
#
# Before you apply this, set up the PingOne environment, as the example in
# resource.tf says, and store the worker app's client secret as the secret
# pingone-acme-eu-p0. The example doesn't create the secret, which keeps the
# client secret out of Terraform's state. Run this where
# pingone-client-secret.txt is:
#
#   gcloud secrets create pingone-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=pingone-client-secret.txt
#
# If the secret exists, store the client secret as a new version instead,
# which is also how to rotate it:
#
#   gcloud secrets versions add pingone-acme-eu-p0 --project my-secrets-project-id \
#     --data-file=pingone-client-secret.txt
#
# Then delete pingone-client-secret.txt.

locals {
  p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b = {
    # The secret that holds the worker app's client secret, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the client secret, add a new version to this secret instead.
    client_secret_project = "my-secrets-project-id"
    client_secret_id      = "pingone-acme-eu-p0"
  }
}

# The connector reads the client secret's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_accessor" {
  project   = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  secret_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_pingone_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_viewer" {
  project   = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  secret_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_pingone_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the client secret.
resource "p0_pingone_aws_iam_federation_app" "gcp_example" {
  id             = "8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b"
  connector      = p0_pingone_aws_iam_federation_connector.gcp_example.id
  region         = "EU"
  environment_id = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
  role_attribute = "awsRoles"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_project
  }
  # The worker app's client ID, from its Configuration tab.
  client_id          = "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"
  client_secret_name = local.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b.client_secret_id

  depends_on = [
    google_secret_manager_secret_iam_member.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_accessor,
    google_secret_manager_secret_iam_member.p0_pingone_aws_iam_federation_app_8e7d6c5b_4a3f_4e2d_9c1b_0a9f8e7d6c5b_viewer,
  ]
}
