terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "6.16.0"
    }
    p0 = {
      source  = "registry.terraform.io/p0-security/p0"
      version = "0.12.0"
    }
  }
}

# Stages the installation of the P0 GCP Security Perimeter 
# To import: terraform import module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.p0_gcp_security_perimeter_staged.security_perimeter_staged p0-demo
resource "p0_gcp_security_perimeter_staged" "security_perimeter_staged" {
  project = var.gcp_project_id
}

# Enable IAM and Cloud Run services
resource "google_project_service" "enable_services" {
  project            = var.gcp_project_id
  for_each = toset([
    "cloudasset.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  service            = each.key
  disable_on_destroy = false
}

# Create P0 Security Perimeter service account
# To import: terraform import module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.google_service_account.security_perimeter_sa projects/p0-demo/serviceAccounts/p0-security-perimeter-sa@p0-demo.iam.gserviceaccount.com
resource "google_service_account" "security_perimeter_sa" {
  project      = var.gcp_project_id
  account_id   = "p0-security-perimeter-sa"
  display_name = "P0 Security Perimeter Service Account"
  description  = "Service account to manage p0 security perimeter"
}

# Deploys P0 Security Perimeter cloud run service
# To import: terraform import 'module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.google_cloud_run_service.security_perimeter' 'us-west1/p0-demo/p0-security-perimeter-prod'
resource "google_cloud_run_service" "security_perimeter" {
  project  = var.gcp_project_id
  name     = "p0-security-perimeter-prod"
  location = var.location

  template {
    spec {
      containers {
        image = "docker.io/p0security/p0-security-perimeter-gcloud:sha-d8092dc"
        env {
          name  = "DOMAIN_ALLOW_PATTERN"
          value = ".*@p0[.]dev"
        }
        env {
          name  = "GCLOUD_PROJECT"
          value = var.p0_project_id
        }
      }
      service_account_name = "${google_service_account.security_perimeter_sa.email}"
    }
  }
}

# Create the P0 Security Perimeter invoker role
# To import: terraform import module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.google_project_iam_custom_role.invoker_role projects/p0-demo/roles/p0SecurityPerimeterInvoker
resource "google_project_iam_custom_role" "invoker_role" {
  project     = var.gcp_project_id
  role_id     = p0_gcp_security_perimeter_staged.security_perimeter_staged.custom_role.id
  title       = p0_gcp_security_perimeter_staged.security_perimeter_staged.custom_role.name
  description = "P0 IAM cloud run invoker role"
  permissions = p0_gcp_security_perimeter_staged.security_perimeter_staged.required_permissions
}

# Grants the P0 service account access to the Security Perimeter invoker role
resource "google_cloud_run_service_iam_member" "invoker_role_binding" {
  project   = var.gcp_project_id
  service   = google_cloud_run_service.security_perimeter.name
  location  = google_cloud_run_service.security_perimeter.location
  role      = google_project_iam_custom_role.invoker_role.id
  member    = "serviceAccount:${var.service_account_email}"
}

# Creates the P0 Security Perimeter reader role
# To import: terraform import module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.google_project_iam_custom_role.reader_role projects/p0-demo/roles/p0SecurityPerimeterReader
resource "google_project_iam_custom_role" "reader_role" {
  role_id     = "p0SecurityPerimeterReader"
  title       = "p0 Security Perimeter Reader"
  description = "Role used by p0 service account to read IAM"
  project     = var.gcp_project_id
  permissions = [
    "resourcemanager.projects.get"
  ]
}

# Grants the P0 service account access to the Security Perimeter reader role
resource "google_project_iam_member" "reader_role_binding" {
  project = var.gcp_project_id
  role    = "${google_project_iam_custom_role.reader_role.name}"
  member    = "serviceAccount:${var.service_account_email}"
}

# Finalizes the installation of the P0 GCP Security Perimeter
# To import: terraform import module.p0_gcp_iam_management.module.p0_gcp_security_perimeter.p0_gcp_security_perimeter.security_perimeter p0-demo
resource "p0_gcp_security_perimeter" "security_perimeter" {
  project         = var.gcp_project_id
  allowed_domains = p0_gcp_security_perimeter_staged.security_perimeter_staged.allowed_domains
  image_digest    = p0_gcp_security_perimeter_staged.security_perimeter_staged.image_digest
  cloud_run_url   = google_cloud_run_service.security_perimeter.status[0].url
  depends_on = [
    google_project_service.enable_services,
    google_service_account.security_perimeter_sa,
    google_cloud_run_service.security_perimeter,
    google_project_iam_custom_role.invoker_role,
    google_cloud_run_service_iam_member.invoker_role_binding,
    google_project_iam_custom_role.reader_role,
    google_project_iam_member.reader_role_binding
  ]
}
