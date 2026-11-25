# The Google Cloud variant of the p0_pingfederate_aws_iam_federation_app example in
# this directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_pingfederate_aws_iam_federation_connector example), and its SCIM password is in
# Google Secret Manager.
#
# Before you apply this, set up Active Directory and PingFederate, as the example in
# resource.tf says, and store the SCIM password as the secret
# pingfederate-acme-gcp-p0. The example doesn't create the secret, which keeps the
# SCIM password out of Terraform's state. Run this where
# pingfederate-scim-password.txt is:
#
#   gcloud secrets create pingfederate-acme-gcp-p0 --project my-secrets-project-id \
#     --data-file=pingfederate-scim-password.txt
#
# If the secret exists, store the SCIM password as a new version instead,
# which is also how to rotate it:
#
#   gcloud secrets versions add pingfederate-acme-gcp-p0 --project my-secrets-project-id \
#     --data-file=pingfederate-scim-password.txt
#
# Then delete pingfederate-scim-password.txt.

locals {
  p0_pingfederate_aws_iam_federation_app_acme_gcp = {
    # The secret that holds the SCIM password, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the SCIM password, add a new version to this secret instead.
    scim_password_project = "my-secrets-project-id"
    scim_password_id      = "pingfederate-acme-gcp-p0"
  }
}

# The connector reads the SCIM password's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_pingfederate_aws_iam_federation_app_acme_gcp_accessor" {
  project   = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
  secret_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_pingfederate_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_pingfederate_aws_iam_federation_app_acme_gcp_viewer" {
  project   = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
  secret_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_pingfederate_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the SCIM password.
resource "p0_pingfederate_aws_iam_federation_app" "gcp_example" {
  id               = "acme-gcp"
  pingfederate_url = "https://sso.example.com:9031"

  scim = {
    connector      = p0_pingfederate_aws_iam_federation_connector.gcp_example.id
    role_attribute = "awsRole"
    scim_username  = "p0-scim"

    vault = {
      type       = "gcp-sm"
      project_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
    }
    scim_password_name = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
    # PingFederate's TLS certificate is from an internal CA. P0 removes the
    # whitespace around the value, so trim the file's contents.
    ca_certificate = trimspace(file("${path.module}/pingfederate-ca.pem"))
  }

  depends_on = [
    google_secret_manager_secret_iam_member.p0_pingfederate_aws_iam_federation_app_acme_gcp_accessor,
    google_secret_manager_secret_iam_member.p0_pingfederate_aws_iam_federation_app_acme_gcp_viewer,
  ]
}
