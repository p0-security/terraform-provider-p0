resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  logs_topic_project = "my-logs-project"
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

# Data access logs are sent to P0 using this Pub/Sub topic
resource "google_pubsub_topic" "example" {
  project = locals.logs_topic_project
  name    = p0_gcp.example.access_logs.pub_sub.topic_id
}

# The log sink that writes to the P0 access-logging Pub/Sub topic
resource "google_logging_organization_sink" "example" {
  org_id           = p0_gcp.example.org_id
  name             = p0_gcp.example.access_logs.logging.sink_id
  include_children = true
  destination      = google_pubsub_topic.example.id
  description      = "P0 data access log sink"

  filter = p0_gcp.example.access_logs.logging.filter
}

# Grants the logging service account permission to write to the access-logging Pub/Sub topic
resource "google_pubsub_topic_iam_member" "logging_example" {
  project = locals.logs_topic_project
  role    = p0_gcp.example.access_logs.logging.role
  topic   = google_pubsub_topic.example.name
  member  = google_logging_organization_sink.example.writer_identity
}

# Grants P0 permission to read from the access-logging Pub/Sub topic
resource "google_pubsub_topic_iam_member" "p0_example" {
  project = locals.logs_topic_project
  role    = p0_gcp.example.access_logs.predefined_role
  topic   = google_pubsub_topic.example.name
  member  = "serviceAccount:${p0_gcp.example.serviceAccountEmail}"
}

# Install organization access logging in P0
resource "p0_gcp_access_logs" "example" {
  topic_project_id = locals.logs_topic_project
  depends_on = [
    google_logging_project_sink.example,
    google_pubsub_topic_iam_member.p0_example
  ]
}
