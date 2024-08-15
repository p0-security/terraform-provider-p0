resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my_project_id"
}

# Follow instructions for creating Terraform for IAM assessment in p0_gcp_iam_assessment documentation
# ...

resource "p0_gcp_iam_assessment" "example" {
  project    = locals.project
  depends_on = [google_project_iam_member.example]
}

resource "google_project_iam_audit_config" "example" {
  project = locals.project
  service = "allServices"
  audit_log_config {
    log_type = "ADMIN_READ"
  }
  audit_log_config {
    log_type = "DATA_READ"
  }
  audit_log_config {
    log_type = "DATA_WRITE"
  }
}

resource "google_project_iam_custom_role" "example" {
  project     = locals.project
  role_id     = p0_gcp.example.access_logs.custom_role.id
  title       = p0_gcp.example.access_logs.custom_role.name
  permissions = p0_gcp.example.access_logs.permissions
}

# Grants the logging service account permission to write to the access-logging Pub/Sub topic
resource "google_project_iam_member" "example" {
  project = locals.project
  role    = google_project_iam_custom_role.example.name
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Finish the P0 access-logs installation
resource "p0_gcp_access_logs" "example" {
  project = locals.project
  depends_on = [
    google_project_iam_audit_config.example,
    google_project_iam_member.example
  ]
}
