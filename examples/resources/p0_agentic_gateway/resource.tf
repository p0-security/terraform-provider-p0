resource "p0_agentic_gateway" "example" {
  id             = "primary"
  url            = "https://gateway.example.com"
  oauth_endpoint = "https://oauth.gateway.example.com"
  log_project_id = "my-gcp-logging-project"
}
