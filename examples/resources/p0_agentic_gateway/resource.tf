# See the p0_agentic_gateway_staged example for the preceding steps
# (staging the gateway, and configuring it to trust the assigned service
# account) that this resource depends on.

resource "p0_agentic_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
  lets_encrypt_email = "admin@example.com"
  oidc_client_id     = "your-upstream-oidc-client-id"
  storage_class      = "gp2"
}

# Uses the p0-security/p0-agentic-gateway-stack/kubernetes module:
# https://github.com/p0-security/terraform-kubernetes-p0-agentic-gateway-stack
module "agentic_gateway_stack" {
  source  = "p0-security/p0-agentic-gateway-stack/kubernetes"
  version = "0.1.10"

  values = [
    yamlencode({
      "agentic-gateway" = {
        agenticGatewayServer = {
          manageAllowedEmails = p0_agentic_gateway_staged.example.service_account_email
        }
      }
    }),
  ]
}

# Finalizes the install; depends_on ensures the gateway trusts P0's service
# account before verification is attempted.
resource "p0_agentic_gateway" "example" {
  id = p0_agentic_gateway_staged.example.id
  domain_hosting = {
    load_balancer_ip = "<your-gateway-loadbalancer-ip>"
  }
  log_project_id = "my-gcp-logging-project"
  depends_on     = [module.agentic_gateway_stack]
}
