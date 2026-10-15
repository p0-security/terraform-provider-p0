# Adds a Harbor instance that P0's Harbor connector on Google Cloud Run reaches,
# with the robot account's secret in Google Secret Manager. For a connector on
# AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_harbor_connector example, which deploys and installs the
# connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the connector's robot account in Harbor, as this
# resource's page describes, and store the file Harbor exports for it as the
# secret harbor-prod-robot. The example doesn't create the secret, which keeps
# the robot account's credentials out of Terraform's state. To store it, with
# gcloud signed in to the secret's project:
#
#   gcloud secrets create harbor-prod-robot --project my-secrets-project \
#     --data-file 'robot$p0-connector.json'

locals {
  harbor_instance = {
    # The instance's address: https and the host, with nothing after it.
    url = "https://harbor.example.com"
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "harbor-prod-robot"
    # Optional: the secret that holds a Harbor admin credential, for the system
    # administrator access. Store it like the robot's, as a JSON object with
    # issuer, clientId, clientSecret and refreshToken for a dedicated identity
    # provider user who is a Harbor system administrator. It is as sensitive as
    # an admin password. It can be added, changed or removed in place: grant the
    # connector read on a new secret first, as below, and revoke open system
    # administrator access in P0 before you remove it.
    admin_secret_name = "harbor-prod-admin"
  }
}

# The connector reads the robot account's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "harbor_accessor" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

resource "google_secret_manager_secret_iam_member" "harbor_viewer" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

# The same two roles on the admin credential's secret.
resource "google_secret_manager_secret_iam_member" "harbor_admin_accessor" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.admin_secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

resource "google_secret_manager_secret_iam_member" "harbor_admin_viewer" {
  project   = local.harbor_instance.secrets_project_id
  secret_id = local.harbor_instance.admin_secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.harbor.email}"
}

# Creating it has P0 check the instance through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_harbor_instance" "example" {
  id        = "prod"
  connector = p0_harbor_connector.example.id
  url       = local.harbor_instance.url

  vault = {
    type       = "gcp-sm"
    project_id = local.harbor_instance.secrets_project_id
  }
  secret_name       = local.harbor_instance.secret_name
  admin_secret_name = local.harbor_instance.admin_secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.harbor_accessor,
    google_secret_manager_secret_iam_member.harbor_viewer,
    google_secret_manager_secret_iam_member.harbor_admin_accessor,
    google_secret_manager_secret_iam_member.harbor_admin_viewer,
  ]
}
