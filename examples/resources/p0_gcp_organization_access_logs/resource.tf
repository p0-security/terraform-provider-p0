resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# The existing Google Cloud project in which P0 will create the access-logs Pub/Sub topic
locals {
  logs_topic_project = "my-project-id"
}

# Enable Data Access audit logging across the entire organization so that P0 can
# collect access logs to enhance IAM assessment
resource "google_organization_iam_audit_config" "example" {
  org_id  = p0_gcp.example.organization_id
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

# Grants P0's service account the permissions needed to create the organization-level
# logging sink and the access-logs Pub/Sub topic
resource "google_organization_iam_member" "example" {
  org_id = p0_gcp.example.organization_id
  role   = google_organization_iam_custom_role.example.name
  member = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Install organization access logging in P0
resource "p0_gcp_organization_access_logs" "example" {
  topic_project_id = local.logs_topic_project
  depends_on = [
    google_organization_iam_audit_config.example,
    google_organization_iam_member.example
  ]
}
