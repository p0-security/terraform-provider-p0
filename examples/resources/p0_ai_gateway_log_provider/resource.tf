# Reads activity from the VictoriaLogs instance that the ai-gateway-stack chart
# runs next to the gateway. It needs no other settings.
resource "p0_ai_gateway_log_provider" "victorialogs" {
  id   = "victorialogs"
  type = "victorialogs"
}

# Reads activity from Google Cloud Logging. project_id is the project that
# holds the log bucket, which may differ from the gateway's own project.
resource "p0_ai_gateway_log_provider" "gcp" {
  id         = "gcp-logs"
  type       = "gcp"
  project_id = "my-gcp-logging-project"
}
