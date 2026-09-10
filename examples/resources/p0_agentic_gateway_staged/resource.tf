# P0 assigns a service account to communicate with your gateway.
resource "p0_agentic_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
  lets_encrypt_email = "admin@example.com"
  oidc_client_id     = "your-upstream-oidc-client-id"
  storage_class      = "gp2"
}

# Your gateway must trust that service account before P0 can finish
# installing (see the p0_agentic_gateway example for the next step). Uses the
# p0-security/p0-agentic-gateway-stack/kubernetes module: https://github.com/p0-security/terraform-kubernetes-p0-agentic-gateway-stack
module "agentic_gateway_stack" {
  source  = "p0-security/p0-agentic-gateway-stack/kubernetes"
  version = "1.0.0"

  release_name             = p0_agentic_gateway_staged.example.id
  namespace                = p0_agentic_gateway_staged.example.kubernetes_namespace
  gateway_url              = p0_agentic_gateway_staged.example.domain_hosting.url
  lets_encrypt_email       = p0_agentic_gateway_staged.example.lets_encrypt_email
  oidc_client_id           = p0_agentic_gateway_staged.example.oidc_client_id
  storage_class            = p0_agentic_gateway_staged.example.storage_class
  p0_url                   = "https://api.p0.app/o/your-org"
  p0_audience              = "https://api.p0.app/o/your-org"
  p0_service_account_email = p0_agentic_gateway_staged.example.service_account_email
}
