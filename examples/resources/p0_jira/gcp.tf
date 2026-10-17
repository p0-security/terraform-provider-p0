# The Google Cloud Run variant of the p0_jira example in resource.tf, for the
# site acme-dev.atlassian.net, with the service account's OAuth client secret in
# Google Secret Manager.
#
# Before you apply this, create the site's Atlassian service account and its
# OAuth credential, store the credential's client secret in Secret Manager as
# jira-acme-dev in the project secrets-project, and install p0_gcp (see its
# example). To store the secret, with the gcloud CLI signed in:
#
#   gcloud secrets create jira-acme-dev \
#     --project secrets-project --data-file=client-secret.txt
#
# The connector's Terraform is what P0's Jira installer generates for the site,
# with p0_jira in place of its output. Copy it from the installer, which pins
# the current connector image.

# P0's Jira connector for the site acme-dev.atlassian.net, on Google Cloud Run.
#
# It needs Terraform's google provider at 6.0 or later, and the Service Usage
# API enabled in project jira-host,
# which lets Terraform enable the IAM and Cloud Run APIs. If the first apply
# fails right after it enables them, apply again: a newly enabled API can take
# a few minutes to answer.
#
# Cloud Run pulls P0's connector image from Docker Hub. Terraform reads the
# image's digest from Docker Hub on every plan, so the machine that runs
# Terraform has to reach it. It needs no Docker.
#
# Other sites can share this configuration, but each needs its own connector
# name: the connector's service and service account are named after it.
#
# The secret has to exist already, holding the client secret. This doesn't
# create it, and never reads or holds its value.

terraform {
  # The image's digest check needs Terraform 1.2.
  required_version = ">= 1.2"

  # If your configuration already has a required_providers block, such as the
  # one in resource.tf, add these to it, and delete this one.
  required_providers {
    p0 = {
      source = "p0-security/p0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 3.3, < 5.0"
    }
  }
}

locals {
  p0_jira_site_acme_dev = {
    # Where P0 invokes the connector. P0's installer has the same values.
    project_id       = "jira-host"
    connector_name   = "p0-jira-site-acme-dev"
    connector_region = "us-west1"
    # P0's own service account, from the p0_gcp example. The connector accepts
    # calls from it alone.
    invoker_email = p0_gcp.example.service_account_email
    # The connector gives Jira access only to users whose whole email domain
    # matches this. It's also p0_jira's domain_pattern, which P0 checks against
    # the connector's. A new pattern replaces the site in P0.
    domain_pattern = "example\\.com"
    # The secret that holds the client secret, and the project it's in. A new
    # secret replaces the site in P0. To rotate the client secret, add a new
    # version to this secret instead.
    secrets_project_id = "secrets-project"
    secret_name        = "jira-acme-dev"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These are placeholders, for an image that doesn't exist. Use
    # the pinned values in the Terraform that P0's installer shows, or the
    # image's published tag and digest.
    image_tag    = "sha-1234567"
    image_digest = "sha256:abababababababababababababababababababababababababababababababab"
  }
}

# Reads the image's digest from Docker Hub. It never talks to a Docker daemon,
# so none has to be running.
provider "docker" {
  alias                       = "p0_jira_site_acme_dev"
  disable_docker_daemon_check = true
}

# Stops the plan if the tag has been moved to another image since P0 pinned
# it, so that only the image P0 pinned is deployed.
data "docker_registry_image" "p0_jira_site_acme_dev" {
  provider = docker.p0_jira_site_acme_dev

  name = "p0security/p0-connector-jira-gcloud:${local.p0_jira_site_acme_dev.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_jira_site_acme_dev.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Jira installer, or contact support@p0.dev."
    }
  }
}

# The IAM and Cloud Run APIs. Never disabled on destroy: other services in the
# project may use them.
resource "google_project_service" "p0_jira_site_acme_dev" {
  for_each = toset(["iam.googleapis.com", "run.googleapis.com"])

  project            = local.p0_jira_site_acme_dev.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account whose ID is the
# connector's name, which P0 names as the site's
# hosting.connector_service_account. Its only role is read on the site's
# secret, below.
#
# Don't destroy and recreate it. Google treats a recreated account as a new
# one, even under the same name: it doesn't get the old account's roles.
# https://cloud.google.com/iam/docs/service-accounts-delete-undelete
resource "google_service_account" "p0_jira_site_acme_dev" {
  project      = local.p0_jira_site_acme_dev.project_id
  account_id   = local.p0_jira_site_acme_dev.connector_name
  display_name = "P0 Jira site connector acme-dev"

  depends_on = [google_project_service.p0_jira_site_acme_dev]
}

# The connector reads the secret's current value, on this secret alone, never
# on its project.
resource "google_secret_manager_secret_iam_member" "p0_jira_site_acme_dev_accessor" {
  project   = local.p0_jira_site_acme_dev.secrets_project_id
  secret_id = local.p0_jira_site_acme_dev.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.p0_jira_site_acme_dev.email}"
}

# The accessor role gives secretmanager.versions.access only. P0's secret
# client reads the latest version's value and then that version's metadata
# (secretmanager.versions.get), which the accessor role doesn't cover. The
# viewer role does, and can change nothing: on this secret it reads the
# secret's metadata, its versions' metadata and its IAM policy, never a value.
resource "google_secret_manager_secret_iam_member" "p0_jira_site_acme_dev_secret_viewer" {
  project   = local.p0_jira_site_acme_dev.secrets_project_id
  secret_id = local.p0_jira_site_acme_dev.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.p0_jira_site_acme_dev.email}"
}

resource "google_cloud_run_v2_service" "p0_jira_site_acme_dev" {
  project             = local.p0_jira_site_acme_dev.project_id
  name                = local.p0_jira_site_acme_dev.connector_name
  location            = local.p0_jira_site_acme_dev.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud; IAM, not origin, gates
  # access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_jira_site_acme_dev.email

    containers {
      image = "docker.io/p0security/p0-connector-jira-gcloud:${local.p0_jira_site_acme_dev.image_tag}@${data.docker_registry_image.p0_jira_site_acme_dev.sha256_digest}"

      # The connector checks every caller's token against this, and refuses
      # every call while it's unset, so it is not redundant with the
      # roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = local.p0_jira_site_acme_dev.invoker_email
      }

      env {
        name  = "DOMAIN_PATTERN"
        value = local.p0_jira_site_acme_dev.domain_pattern
      }
    }
  }

  depends_on = [google_project_service.p0_jira_site_acme_dev]
}

# P0 invokes the connector as its own service account. Nothing here lets
# anyone else invoke it.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_site_acme_dev_invoker" {
  project  = local.p0_jira_site_acme_dev.project_id
  location = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  name     = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.p0_jira_site_acme_dev.invoker_email}"
}

# P0 reads the service to get its invocation URL when it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_site_acme_dev_viewer" {
  project  = local.p0_jira_site_acme_dev.project_id
  location = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  name     = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${local.p0_jira_site_acme_dev.invoker_email}"
}

# Installs Jira for the site. Creating it has P0 check the site through the
# connector, so it waits until P0 can invoke the connector and read its service,
# and the connector can read the client secret. A new grant can take a minute or
# two to take effect, so if the first check is denied access, apply again.
resource "p0_jira" "gcp_example" {
  site = "acme-dev.atlassian.net"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_jira_site_acme_dev.project_id
    connector_name   = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
    connector_region = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  }

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_jira_site_acme_dev.secrets_project_id
  }
  client_secret_name = local.p0_jira_site_acme_dev.secret_name
  domain_pattern     = local.p0_jira_site_acme_dev.domain_pattern

  # The client ID of the service account's OAuth credential. A new credential
  # updates the site in place.
  client_id = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_jira_site_acme_dev_invoker,
    google_cloud_run_v2_service_iam_member.p0_jira_site_acme_dev_viewer,
    google_secret_manager_secret_iam_member.p0_jira_site_acme_dev_accessor,
    google_secret_manager_secret_iam_member.p0_jira_site_acme_dev_secret_viewer,
  ]
}
