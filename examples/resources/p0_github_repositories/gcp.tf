# The Google Cloud variant of the p0_github_repositories example in this
# directory, for another organization, my-other-github-org: P0's GitHub
# Repositories connector on Cloud Run, and the GitHub App's private key in Google
# Secret Manager. resource.tf's terraform block lists the providers both use.
#
# Before you apply this, create the organization's GitHub App, store its private
# key in Secret Manager as github-my-other-github-org-private-key, and install
# p0_gcp. This resource's page lists what the App needs. The example doesn't
# create the key's secret, which keeps the key out of Terraform's state.
#
# The connector's Terraform is what P0's GitHub Repositories installer generates
# for the organization, with p0_github_repositories in place of its output. Copy
# it from the installer, which pins the current connector image.

# P0's GitHub Repositories connector for my-other-github-org, on Google Cloud Run.
#
# Applying this deploys P0's connector image from Docker Hub as a Cloud Run
# service in project my-project-id. Run terraform with Google credentials that can
# manage Cloud Run, service accounts and IAM in project my-project-id, and IAM on
# the private key's secret in project my-project-id.
#
# P0 runs one connector for each organization, so apply this once, in one
# cloud. Other organizations' connectors can share this configuration, but each
# needs its own connector name: the connector's service and service account are
# named after it, and these names are unique in a Google Cloud project.

locals {
  p0_github_repositories_my_other_github_org = {
    # Where P0 invokes the connector. P0's installer has the same values.
    project_id       = "my-project-id"
    connector_name   = "p0-github-my-other-github-org"
    connector_region = "us-central1"
    # The secret that holds the GitHub App's private key, by its name in its
    # project. P0 gives the connector its resource name,
    # projects/my-project-id/secrets/github-my-other-github-org-private-key.
    # The connector can read this secret and no other.
    private_key_secret_id = "github-my-other-github-org-private-key"
    secrets_project_id    = "my-project-id"
    # The connector's own service account, named after the connector.
    service_account_id = "p0-github-my-other-gi-e11606a2"
    # P0's service account, which invokes the connector.
    p0_service_account_email = "p0-example@p0-prod.iam.gserviceaccount.com"
    # P0's connector image (sha-1234567), pinned by its digest. This is a
    # placeholder, for an image that doesn't exist. Use the pinned image in the
    # Terraform that P0's installer shows, or the image's published digest.
    image = "docker.io/p0security/p0-connector-github-repositories-gcloud@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

resource "google_project_service" "p0_github_repositories_my_other_github_org" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
  ])

  project            = local.p0_github_repositories_my_other_github_org.project_id
  service            = each.key
  disable_on_destroy = false
}

resource "google_service_account" "p0_github_repositories_my_other_github_org" {
  project      = local.p0_github_repositories_my_other_github_org.project_id
  account_id   = local.p0_github_repositories_my_other_github_org.service_account_id
  display_name = "P0 GitHub Repositories connector for my-other-github-org"

  depends_on = [google_project_service.p0_github_repositories_my_other_github_org]
}

# The connector reads the private key's value (secretmanager.versions.access),
# then its version (secretmanager.versions.get). The Secret Accessor role has
# only the first, so the Viewer role is granted too, on this one secret.
resource "google_secret_manager_secret_iam_member" "p0_github_repositories_my_other_github_org_accessor" {
  project   = local.p0_github_repositories_my_other_github_org.secrets_project_id
  secret_id = local.p0_github_repositories_my_other_github_org.private_key_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.p0_github_repositories_my_other_github_org.email}"
}

resource "google_secret_manager_secret_iam_member" "p0_github_repositories_my_other_github_org_viewer" {
  project   = local.p0_github_repositories_my_other_github_org.secrets_project_id
  secret_id = local.p0_github_repositories_my_other_github_org.private_key_secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.p0_github_repositories_my_other_github_org.email}"
}

# No vpc_access: the connector calls api.github.com over the internet.
resource "google_cloud_run_v2_service" "p0_github_repositories_my_other_github_org" {
  project             = local.p0_github_repositories_my_other_github_org.project_id
  name                = local.p0_github_repositories_my_other_github_org.connector_name
  location            = local.p0_github_repositories_my_other_github_org.connector_region
  description         = "P0 GitHub Repositories connector for my-other-github-org"
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. IAM, not the network,
  # decides who can invoke it.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_github_repositories_my_other_github_org.email

    containers {
      image = local.p0_github_repositories_my_other_github_org.image

      # The connector checks each caller's identity token against this
      # account, on top of Cloud Run's roles/run.invoker check.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = local.p0_github_repositories_my_other_github_org.p0_service_account_email
      }
    }
  }

  depends_on = [google_project_service.p0_github_repositories_my_other_github_org]
}

# P0 invokes the connector as its service account, and needs nothing else on
# it.
resource "google_cloud_run_v2_service_iam_member" "p0_github_repositories_my_other_github_org_invoker" {
  project  = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.project
  location = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.location
  name     = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.p0_github_repositories_my_other_github_org.p0_service_account_email}"
}

# P0 reads the service to find its URL when it installs. The invoker role can't
# read it.
resource "google_cloud_run_v2_service_iam_member" "p0_github_repositories_my_other_github_org_viewer" {
  project  = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.project
  location = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.location
  name     = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${local.p0_github_repositories_my_other_github_org.p0_service_account_email}"
}

# Installs GitHub Repositories for the organization. Creating it has P0 check
# the install through the connector, so it waits until P0 can invoke and read
# the service and the connector can read the key. P0 looks up the service's URL
# itself, and hosting.connector_service_uri reads it back. IAM changes can take
# a few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_github_repositories" "gcp_example" {
  org = "my-other-github-org"
  # This organization's own GitHub App, not the one in resource.tf.
  app_id = "234567"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_github_repositories_my_other_github_org.secrets_project_id
  }
  private_key_secret_name = local.p0_github_repositories_my_other_github_org.private_key_secret_id

  hosting = {
    type             = "gcp"
    project_id       = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.project
    connector_name   = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.name
    connector_region = google_cloud_run_v2_service.p0_github_repositories_my_other_github_org.location
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_github_repositories_my_other_github_org_invoker,
    google_cloud_run_v2_service_iam_member.p0_github_repositories_my_other_github_org_viewer,
    google_secret_manager_secret_iam_member.p0_github_repositories_my_other_github_org_accessor,
    google_secret_manager_secret_iam_member.p0_github_repositories_my_other_github_org_viewer,
  ]
}
