# Adds an Argo CD instance that P0's Argo CD connector on Google Cloud Run
# reaches, with the p0-connector account's API token in Google Secret Manager.
# For a connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_argocd_connector example, which deploys and installs the
# connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the p0-connector account in Argo CD, as this
# resource's page describes, and store its API token as the secret
# argocd-prod-token, in the JSON {"token": "<api token>"}. The example doesn't
# create the secret, which keeps the token out of Terraform's state. To store
# it, with the argocd CLI signed in as an admin and gcloud signed in to the
# secret's project:
#
#   argocd account generate-token --account p0-connector |
#     jq -Rc '{token: .}' |
#     gcloud secrets create argocd-prod-token --project my-secrets-project \
#       --data-file -

locals {
  argocd_instance = {
    # The instance's address: https and the host, with nothing after it.
    url = "https://argocd.example.com"
    # The Application that manages argocd-rbac-cm, as <project>/<application>.
    managing_app = "infra/argocd"
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "argocd-prod-token"
  }
}

# The connector reads the API token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "argocd_accessor" {
  project   = local.argocd_instance.secrets_project_id
  secret_id = local.argocd_instance.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.argocd.email}"
}

resource "google_secret_manager_secret_iam_member" "argocd_viewer" {
  project   = local.argocd_instance.secrets_project_id
  secret_id = local.argocd_instance.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.argocd.email}"
}

# Creating it has P0 check the instance through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_argocd_instance" "example" {
  id           = "prod"
  connector    = p0_argocd_connector.example.id
  url          = local.argocd_instance.url
  managing_app = local.argocd_instance.managing_app

  vault = {
    type       = "gcp-sm"
    project_id = local.argocd_instance.secrets_project_id
  }
  secret_name = local.argocd_instance.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.argocd_accessor,
    google_secret_manager_secret_iam_member.argocd_viewer,
  ]
}
