# Adds a LaunchDarkly account that P0's LaunchDarkly connector on Google Cloud
# Run reaches, with its access token's secret in Google Secret Manager. For a
# connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_launchdarkly_connector example, which deploys and installs
# the connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the account's access token, as this resource's
# page describes: a service token on Enterprise, or an admin's personal token
# on any other plan. Store it, and nothing else, as the secret
# launchdarkly-acme-prod-token. The example doesn't create the secret, which
# keeps the token out of Terraform's state. To store it, with the token alone
# in token.txt and gcloud signed in to the secret's project:
#
#   gcloud secrets create launchdarkly-acme-prod-token --project my-secrets-project \
#     --data-file token.txt

locals {
  launchdarkly_account = {
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "launchdarkly-acme-prod-token"
  }
}

# The connector reads the access token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "launchdarkly_accessor" {
  project   = local.launchdarkly_account.secrets_project_id
  secret_id = local.launchdarkly_account.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.launchdarkly.email}"
}

resource "google_secret_manager_secret_iam_member" "launchdarkly_viewer" {
  project   = local.launchdarkly_account.secrets_project_id
  secret_id = local.launchdarkly_account.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.launchdarkly.email}"
}

# Creating it has P0 check the account through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_launchdarkly_account" "example" {
  # Requests name the account by this, as in acme-prod/writer.
  id        = "acme-prod"
  connector = p0_launchdarkly_connector.example.id
  # commercial, eu or federal.
  instance = "commercial"
  # enterprise, or non-enterprise for any other plan.
  plan = "enterprise"

  vault = {
    type       = "gcp-sm"
    project_id = local.launchdarkly_account.secrets_project_id
  }
  token_secret_name = local.launchdarkly_account.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.launchdarkly_accessor,
    google_secret_manager_secret_iam_member.launchdarkly_viewer,
  ]
}
