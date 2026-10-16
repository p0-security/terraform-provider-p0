# Installs P0's Jira connector on Google Cloud Run.
#
# Full chain: p0_gcp (P0's service account) -> p0_jira_ai_connector_staged ->
# the connector's service account and Cloud Run service -> roles/run.invoker
# and roles/run.viewer for P0's service account -> p0_jira_ai_connector ->
# p0_jira_ai_site -> p0_jira_ai_pool_account.
#
# The connector's Terraform is what P0's Jira installer generates for the
# connector, with p0_jira_ai_connector_staged before it and
# p0_jira_ai_connector in place of its output. Copy it from the installer,
# which pins the current connector image.
#
# The service account has no role here: each site that uses the connector
# grants it what it needs in the site's vault project (see the
# p0_jira_ai_site example).

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned, and the image's digest check, need Terraform 1.2.
  required_version = ">= 1.2"

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

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  p0_jira_ai_connector_prod = {
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. Use the pinned values in the Terraform that P0's installer
    # shows.
    image_tag    = "sha-1234567"
    image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

# Tells P0 where the connector will run. None of these can change once the
# connector is added. P0 answers with the service account the connector must
# run as, named after the connector.
resource "p0_jira_ai_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-jira-prod"
    connector_region = "us-central1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  jira = p0_jira_ai_connector_staged.example.hosting
}

# Reads the image's digest from Docker Hub. It never talks to a Docker daemon,
# so none has to be running.
provider "docker" {
  alias                       = "p0_jira_ai_connector_prod"
  disable_docker_daemon_check = true
}

# Stops the plan if the tag has been moved to another image since P0 pinned it,
# so that only the image P0 pinned is deployed.
data "docker_registry_image" "p0_jira_ai_connector_prod" {
  provider = docker.p0_jira_ai_connector_prod

  name = "p0security/p0-connector-jira-ai-gcloud:${local.p0_jira_ai_connector_prod.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_jira_ai_connector_prod.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Jira installer, or contact support@p0.dev."
    }
  }
}

# The IAM, Cloud Run and Secret Manager APIs. Never disabled on destroy: other
# services in the project may use them.
resource "google_project_service" "p0_jira_ai_connector_prod" {
  for_each = toset(["iam.googleapis.com", "run.googleapis.com", "secretmanager.googleapis.com"])

  project            = local.jira.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 named when the
# connector was staged. Don't destroy and recreate it: Google treats a
# recreated account as a new one, which loses each site's grants.
resource "google_service_account" "p0_jira_ai_connector_prod" {
  project      = local.jira.project_id
  account_id   = split("@", local.jira.connector_service_account)[0]
  display_name = "P0 Jira connector prod"

  depends_on = [google_project_service.p0_jira_ai_connector_prod]
}

resource "google_cloud_run_v2_service" "p0_jira_ai_connector_prod" {
  project             = local.jira.project_id
  name                = local.jira.connector_name
  location            = local.jira.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud. IAM, not origin, gates
  # access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_jira_ai_connector_prod.email

    containers {
      image = "docker.io/p0security/p0-connector-jira-ai-gcloud:${local.p0_jira_ai_connector_prod.image_tag}@${data.docker_registry_image.p0_jira_ai_connector_prod.sha256_digest}"

      # The connector checks every caller's token against this, and refuses
      # every call while it's unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }
    }
  }

  depends_on = [google_project_service.p0_jira_ai_connector_prod]
}

# P0 invokes the connector as its own service account. Nothing here lets
# anyone else invoke it.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_ai_connector_prod_invoker" {
  project  = local.jira.project_id
  location = google_cloud_run_v2_service.p0_jira_ai_connector_prod.location
  name     = google_cloud_run_v2_service.p0_jira_ai_connector_prod.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL when it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_ai_connector_prod_viewer" {
  project  = local.jira.project_id
  location = google_cloud_run_v2_service.p0_jira_ai_connector_prod.location
  name     = google_cloud_run_v2_service.p0_jira_ai_connector_prod.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 find the service, check that it
# runs as the staged service account, and invoke it, so it waits until P0's
# service account may do both.
resource "p0_jira_ai_connector" "example" {
  id = p0_jira_ai_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.jira.type
    project_id       = local.jira.project_id
    connector_name   = local.jira.connector_name
    connector_region = local.jira.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_jira_ai_connector_prod_invoker,
    google_cloud_run_v2_service_iam_member.p0_jira_ai_connector_prod_viewer,
  ]
}
