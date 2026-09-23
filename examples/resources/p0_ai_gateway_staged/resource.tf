# P0 assigns a service account to communicate with your gateway.
resource "p0_ai_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
  lets_encrypt_email = "admin@example.com"
  oidc_client_id     = "your-upstream-oidc-client-id"
  storage_class      = "gp2"
}

# Your gateway must trust that service account before P0 can finish
# installing (see the p0_ai_gateway example for the next step). Uses the
# p0-security/p0-ai-gateway-stack/kubernetes module: https://github.com/p0-security/terraform-kubernetes-p0-ai-gateway-stack
module "ai_gateway_stack" {
  source  = "p0-security/p0-ai-gateway-stack/kubernetes"
  version = "0.3.0"

  values = [
    yamlencode({
      "agentic-gateway" = {
        agenticGatewayServer = {
          manageAllowedEmails = p0_ai_gateway_staged.example.service_account_email
        }
      }
    }),
  ]
}
