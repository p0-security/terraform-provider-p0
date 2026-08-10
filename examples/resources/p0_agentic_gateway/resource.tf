# See the p0_agentic_gateway_staged example for the preceding steps
# (staging the gateway, and configuring it to trust the assigned service
# account) that this resource depends on.

resource "p0_agentic_gateway_staged" "example" {
  id  = "primary"
  url = "https://gateway.example.com"
}

resource "helm_release" "oauthed_mcp" {
  name  = "oauthed-mcp"
  chart = "oci://registry-1.docker.io/p0security/p0-helm-oauthed-mcp"

  set = [{
    name  = "oauthed-mcp.mcpServer.manageAllowedEmails"
    value = p0_agentic_gateway_staged.example.service_account_email
  }]
}

# Finalizes the install; depends_on ensures the gateway trusts P0's service
# account before verification is attempted.
resource "p0_agentic_gateway" "example" {
  id             = p0_agentic_gateway_staged.example.id
  oauth_endpoint = "https://oauth.gateway.example.com"
  log_project_id = "my-gcp-logging-project"
  depends_on     = [helm_release.oauthed_mcp]
}
