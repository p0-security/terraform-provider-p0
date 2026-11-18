# Installs a Google Cloud Logging log source for your whole Google Cloud
# organization: P0's connector on Cloud Run, which reads the Cloud Audit Logs of
# every project in the organization that p0_gcp is installed for, so that Access
# Logs can show what each request's access was used for. A project's own log
# source (see p0_gcp_cloud_logging) takes precedence for that project.
#
# Full chain: p0_gcp (see its example) ->
# p0_gcp_organization_cloud_logging_staged -> the connector's service account,
# its organization-wide roles/logging.privateLogViewer binding and its Cloud Run
# service -> roles/run.invoker and roles/run.viewer for P0's service account ->
# p0_gcp_organization_cloud_logging.
#
# The host project needs the Service Usage API enabled. Cloud Run pulls the
# connector image from Docker Hub; Terraform checks its digest there on every
# plan.

terraform {
  # Reading the staged resource's attributes while it is still planned needs
  # Terraform 1.2.
  required_version = ">= 1.2"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 3.3, < 5.0"
    }
    # 0.98.0 added the log-source resources.
    p0 = {
      source  = "p0-security/p0"
      version = ">= 0.98.0"
    }
  }
}

locals {
  # The project the connector runs in.
  host_project_id = "my-connector-project"

  # P0's connector image: its tag on Docker Hub, and the digest that tag must
  # resolve to. To update, take both from the latest version of this example in
  # the Terraform registry: they change together.
  connector_image_name   = "p0security/p0-connector-session-logging-gcp"
  connector_image_tag    = "sha-a266a1e"
  connector_image_digest = "sha256:cd33a3c9149e9318d461e9040756256ebe54aa2cf0a4c8b59a3016d218466f7c"
}

# Staging records the log source in P0, which answers with the connector's
# service name, service account and region.
resource "p0_gcp_organization_cloud_logging_staged" "example" {
  host_project_id = local.host_project_id

  depends_on = [p0_gcp.example]
}

locals {
  log_source = p0_gcp_organization_cloud_logging_staged.example
}

# Reads the image's digest from Docker Hub. It never talks to a Docker daemon.
provider "docker" {
  disable_docker_daemon_check = true
}

# Stops the plan if the tag has been moved to another image since P0 pinned it,
# so that only the image P0 pinned is deployed.
data "docker_registry_image" "connector" {
  name = "${local.connector_image_name}:${local.connector_image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.connector_image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Take both from the latest p0_gcp_organization_cloud_logging example in P0's Terraform registry docs, or contact support@p0.dev."
    }
  }
}

# The IAM and Cloud Run APIs. Never disabled on destroy: other services in the
# project may use them.
resource "google_project_service" "connector" {
  for_each = toset(["iam.googleapis.com", "run.googleapis.com"])

  project            = local.host_project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when it
# staged the log source.
resource "google_service_account" "connector" {
  project      = local.host_project_id
  account_id   = split("@", local.log_source.connector_service_account)[0]
  display_name = "P0 Google Cloud Logging connector"

  depends_on = [google_project_service.connector]
}

# The connector reads every project's Cloud Audit Logs in your organization,
# Data Access logs included.
resource "google_organization_iam_member" "connector_log_viewer" {
  org_id = p0_gcp.example.organization_id
  role   = "roles/logging.privateLogViewer"
  member = "serviceAccount:${google_service_account.connector.email}"
}

# No VPC access: the connector calls only Google's public APIs.
resource "google_cloud_run_v2_service" "connector" {
  project             = local.host_project_id
  name                = local.log_source.connector_service_name
  location            = local.log_source.region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. IAM, not the caller's
  # network, controls access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.connector.email

    containers {
      image = "docker.io/${local.connector_image_name}:${local.connector_image_tag}@${data.docker_registry_image.connector.sha256_digest}"

      # The connector checks every caller's token against this, and refuses
      # every call while it's unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.connector]
}

# P0 invokes the connector as its own service account.
resource "google_cloud_run_v2_service_iam_member" "p0_invoker" {
  project  = local.host_project_id
  location = google_cloud_run_v2_service.connector.location
  name     = google_cloud_run_v2_service.connector.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to find its URL when it installs. The invoker role can't
# do this read.
resource "google_cloud_run_v2_service_iam_member" "p0_viewer" {
  project  = local.host_project_id
  location = google_cloud_run_v2_service.connector.location
  name     = google_cloud_run_v2_service.connector.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. P0 finds the connector's URL and checks that it can read
# the host project's Cloud Audit Logs. A new IAM binding can take a minute to take
# effect, so if the first apply is denied access, apply again.
resource "p0_gcp_organization_cloud_logging" "example" {
  host_project_id = local.log_source.host_project_id

  depends_on = [
    google_organization_iam_member.connector_log_viewer,
    google_cloud_run_v2_service_iam_member.p0_invoker,
    google_cloud_run_v2_service_iam_member.p0_viewer,
  ]
}
