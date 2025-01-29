resource "p0_gcp_security_perimeter_staged" "p0-dev-account" {
  project = local.dev_project_id
}

# Enable iam and cloud run services
resource "google_project_service" "enable_services" {
  for_each = toset([
    "cloudasset.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  service            = each.key
  project            = var.project_id
  disable_on_destroy = false
}

# Create the service account for the p0 security perimeter
resource "google_service_account" "p0_security_perimeter_sa" {
  account_id   = "p0-security-perimeter-sa"
  display_name = "P0 Security Perimeter Service Account"
  description  = "Service account to manage p0 security perimeter"
  project      = var.project_id
}

# deploys p0 security perimeter cloud run service
resource "google_cloud_run_service" "p0_security_perimeter" {
  name     = "p0-security-perimeter-dev"
  project  = var.project_id
  location = var.location

  template {
    spec {
      containers {
        image = "docker.io/p0security/p0-security-perimeter-gcloud:sha-d8092dc"
        env {
          name  = "DOMAIN_ALLOW_PATTERN"
          value = ".*@(p0[.]dev|nathanbrahmsp0[.]onmicrosoft[.]com|permz[.]us)"
        }
        env {
          name  = "GCLOUD_PROJECT"
          value = var.project_id
        }
      }
      service_account_name = google_service_account.p0_security_perimeter_sa.email
    }
  }
}

# Create the p0 security perimeter invoker role, this is assumed by p0 service account
resource "google_project_iam_custom_role" "p0_security_perimeter_invoker_role" {
  role_id     = p0_gcp_security_perimeter_staged.p0-dev-account.custom_role.id
  title       = p0_gcp_security_perimeter_staged.p0-dev-account.custom_role.name
  description = "P0 IAM cloud run invoker role"
  project     = var.project_id
  permissions = p0_gcp_security_perimeter_staged.p0-dev-account.required_permissions
}

# Grants the p0 service account access to the role just created
resource "google_cloud_run_service_iam_member" "invoker_access" {
  service  = google_cloud_run_service.p0_security_perimeter.name
  location = google_cloud_run_service.p0_security_perimeter.location
  role     = "projects/p0-gcp-project/roles/p0SecurityPerimeterInvoker"
  member   = "serviceAccount:customer-p0-gcp-sa@p0-gcp-project.iam.gserviceaccount.com"
}

resource "p0_gcp_security_perimeter" "p0-dev-account" {
  project         = var.project_id
  allowed_domains = p0_gcp_security_perimeter_staged.p0-dev-account.allowed_domains
  image_digest    = p0_gcp_security_perimeter_staged.p0-dev-account.image_digest
  cloud_run_url   = google_cloud_run_service.p0_security_perimeter.status[0].url
  depends_on = [
    google_project_service.enable_services,
    google_service_account.p0_security_perimeter_sa,
    google_cloud_run_service.p0_security_perimeter,
    google_project_iam_custom_role.p0_security_perimeter_invoker_role,
    google_cloud_run_service_iam_member.invoker_access
  ]
}