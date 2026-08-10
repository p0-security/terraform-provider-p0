# P0 assigns a service account to communicate with your gateway.
resource "p0_agentic_gateway_staged" "example" {
  id  = "primary"
  url = "https://gateway.example.com"
}

# Your gateway must trust that service account before P0 can finish
# installing (see the p0_agentic_gateway example for the next step).
resource "helm_release" "oauthed_mcp" {
  name  = "oauthed-mcp"
  chart = "oci://registry-1.docker.io/p0security/p0-helm-oauthed-mcp"

  set = [{
    name  = "oauthed-mcp.mcpServer.manageAllowedEmails"
    value = p0_agentic_gateway_staged.example.service_account_email
  }]
}
