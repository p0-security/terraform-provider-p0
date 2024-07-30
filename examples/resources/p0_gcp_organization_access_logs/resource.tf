resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "google_organization_iam_audit_config" "example" {
  org_id  = p0_gcp.example.org_id
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

resource "google_organization_iam_custom_role" "example" {
  org_id      = p0_gcp.example.organization_id
  role_id     = p0_gcp.example.access_logs.custom_role.id
  title       = p0_gcp.example.access_logs.custom_role.name
  permissions = p0_gcp.example.access_logs.permissions
}

# Grants the logging service account permission to write to the access-logging Pub/Sub topic
resource "google_organization_iam_member" "example" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.example.name
  member = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Install organization access logging in P0
resource "p0_gcp_access_logs" "example" {
  topic_project_id = locals.logs_topic_project
  depends_on = [
    google_organization_iam_audit_config.example,
    google_organization_iam_member.example
  ]
}
