# See the p0_agentic_gateway_staged example for the preceding steps
# (staging the gateway, and configuring it to trust the assigned service
# account) that this resource depends on.

resource "p0_agentic_gateway_staged" "example" {
  id  = "primary"
  url = "https://gateway.example.com"
}

# Uses the p0-security/oauthed-mcp/kubernetes module:
# https://github.com/p0-security/terraform-kubernetes-p0-oauthed-mcp
module "oauthed_mcp" {
  source  = "p0-security/oauthed-mcp/kubernetes"
  version = "0.1.9"

  values = [
    yamlencode({
      "oauthed-mcp" = {
        mcpServer = {
          manageAllowedEmails = p0_agentic_gateway_staged.example.service_account_email
        }
      }
    }),
  ]
}

# Finalizes the install; depends_on ensures the gateway trusts P0's service
# account before verification is attempted.
resource "p0_agentic_gateway" "example" {
  id             = p0_agentic_gateway_staged.example.id
  url            = p0_agentic_gateway_staged.example.url
  oauth_endpoint = "https://oauth.gateway.example.com"
  log_project_id = "my-gcp-logging-project"
  depends_on     = [module.oauthed_mcp]
}
