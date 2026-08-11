# P0 assigns a service account to communicate with your gateway.
resource "p0_agentic_gateway_staged" "example" {
  id  = "primary"
  url = "https://gateway.example.com"
}

# Your gateway must trust that service account before P0 can finish
# installing (see the p0_agentic_gateway example for the next step). Uses the
# p0-security/oauthed-mcp/kubernetes module: https://github.com/p0-security/terraform-kubernetes-p0-oauthed-mcp
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
