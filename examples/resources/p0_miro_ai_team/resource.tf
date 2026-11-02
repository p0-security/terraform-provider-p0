# Adds a Miro team that P0's Miro (AI) connector on Google Cloud Run reaches,
# with the connector user's access token secret in Google Secret Manager. For a
# connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_miro_ai_connector example, which deploys and installs the
# connector. Apply the connector before the team. Google Secret Manager must
# already be installed in P0 for the secret's project: this provider has no
# resource for it, so install it in the P0 app.
#
# Before you apply this, create the connector user's access token, as this
# resource's page describes: from a private Miro app with the boards:read,
# boards:write and team:read scopes. Store it, and nothing else, as the secret
# miro-design-token. The example doesn't create the secret, which keeps the
# token out of Terraform's state. To store it, with the token alone in
# token.txt and gcloud signed in to the secret's project:
#
#   gcloud secrets create miro-design-token --project my-secrets-project \
#     --data-file token.txt

locals {
  miro_ai_team = {
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "miro-design-token"
  }
}

# The connector reads the access token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "miro_ai_accessor" {
  project   = local.miro_ai_team.secrets_project_id
  secret_id = local.miro_ai_team.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.miro_ai.email}"
}

resource "google_secret_manager_secret_iam_member" "miro_ai_viewer" {
  project   = local.miro_ai_team.secrets_project_id
  secret_id = local.miro_ai_team.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.miro_ai.email}"
}

# Creating it has P0 check the team through the connector, so it waits until
# the connector is installed and the grants exist. IAM changes can take a few
# seconds to take effect, so if the first check is denied access, apply again.
resource "p0_miro_ai_team" "example" {
  # Requests name the team by this, as in board design <board-id> viewer.
  id        = "design"
  connector = p0_miro_ai_connector.example.id

  vault = {
    type       = "gcp-sm"
    project_id = local.miro_ai_team.secrets_project_id
  }
  token_secret_name = local.miro_ai_team.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.miro_ai_accessor,
    google_secret_manager_secret_iam_member.miro_ai_viewer,
  ]
}
