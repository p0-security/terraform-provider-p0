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

# Data access logs are sent to P0 using this Pub/Sub topic
resource "google_pubsub_topic" "example" {
  project = locals.project
  name    = p0_gcp.example.access_logs.pub_sub.topic_id
}

# The log sink that writes to the P0 access-logging Pub/Sub topic
resource "google_logging_project_sink" "example" {
  project     = locals.project
  name        = p0_gcp.example.access_logs.logging.sink_id
  destination = "pubsub.googleapis.com/projects/my_project/topics/${google_pubsub_topic.example.name}"
  description = "P0 data access log sink"

  filter = p0_gcp.example.access_logs.logging.filter
}

# Grants the logging service account permission to write to the access-logging Pub/Sub topic
resource "google_pubsub_topic_iam_member" "logging_example" {
  project = locals.project
  role    = p0_gcp.example.access_logs.logging.role
  topic   = google_pubsub_topic.example.name
  member  = "your logging service account email"
}

# Grants P0 permission to read from the access-logging Pub/Sub topic
resource "google_pubsub_topic_iam_member" "p0_example" {
  project = locals.project
  role    = p0_gcp.example.access_logs.predefined_role
  topic   = google_pubsub_topic.example.name
  member  = "serviceAccount:${p0_gcp.example.serviceAccountEmail}"
}

# Finish the P0 access-logs installation
resource "p0_gcp_access_logs" "example" {
  project = locals.project
  depends_on = [
    google_logging_project_sink.example,
    google_pubsub_topic_iam_member.p0_example
  ]
}