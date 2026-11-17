# See the p0_ai_gateway example for the gateway install.
#
# P0 checks that it can reach a gateway each time its link changes, so the
# gateway must be running. Each gateway has one link.

# A gateway that reads activity from the VictoriaLogs instance next to it.
resource "p0_ai_gateway_log_provider" "victorialogs" {
  id   = "victorialogs"
  type = "victorialogs"
}

resource "p0_ai_gateway_log_provider_link" "example" {
  gateway      = p0_ai_gateway.example.id
  log_provider = p0_ai_gateway_log_provider.victorialogs.id
}

# A second gateway whose logs go to Google Cloud Logging. project_id is the
# project that holds the log bucket.
resource "p0_ai_gateway_log_provider" "gcp" {
  id         = "gcp-logs"
  type       = "gcp"
  project_id = "my-gcp-logging-project"
}

resource "p0_ai_gateway_log_provider_link" "gcp_example" {
  gateway      = p0_ai_gateway.gcp_example.id
  log_provider = p0_ai_gateway_log_provider.gcp.id
}
