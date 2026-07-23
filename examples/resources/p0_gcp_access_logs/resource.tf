resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# Access-log collection requires p0_gcp_iam_assessment first; that chain is inlined below (see its example).
resource "p0_gcp_iam_assessment_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Role granting P0 read of project IAM config and asset inventory.
resource "google_project_iam_custom_role" "iam_assessment" {
  project     = local.project
  role_id     = p0_gcp_iam_assessment_staged.example.custom_role.id
  title       = p0_gcp_iam_assessment_staged.example.custom_role.name
  description = "Integration role for P0 IAM assessment integration"
  permissions = p0_gcp_iam_assessment_staged.example.permissions
}

resource "google_project_iam_member" "iam_assessment" {
  project = local.project
  role    = google_project_iam_custom_role.iam_assessment.name
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# p0_gcp_iam_assessment fails validation unless installed after the grant above.
resource "p0_gcp_iam_assessment" "example" {
  project    = local.project
  depends_on = [google_project_iam_member.iam_assessment]
}

# Enable audit logging so P0 can collect access logs for this project.
resource "google_project_iam_audit_config" "example" {
  project = local.project
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

# Role granting P0 permission to create the access-log sink infrastructure.
resource "google_project_iam_custom_role" "access_logs" {
  project     = local.project
  role_id     = p0_gcp.example.access_logs.custom_role.id
  title       = p0_gcp.example.access_logs.custom_role.name
  permissions = p0_gcp.example.access_logs.permissions
}

resource "google_project_iam_member" "access_logs" {
  project = local.project
  role    = google_project_iam_custom_role.access_logs.name
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

resource "p0_gcp_access_logs" "example" {
  project = local.project
  depends_on = [
    p0_gcp_iam_assessment.example,
    google_project_iam_audit_config.example,
    google_project_iam_member.access_logs
  ]
}
