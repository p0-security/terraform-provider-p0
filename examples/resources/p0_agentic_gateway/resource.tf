# See the p0_agentic_gateway_staged example for the preceding steps
# (staging the gateway, and configuring it to trust the assigned service
# account) that this resource depends on.

resource "p0_agentic_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
  lets_encrypt_email   = "admin@example.com"
  oidc_client_id       = "your-upstream-oidc-client-id"
  storage_class        = "gp2"
  kubernetes_namespace = "p0-agentic-gateway"
}

locals {
  # The hostname only, so a URL carrying credentials, a port, a path, a query
  # or a fragment still satisfies the module's gateway_url validation. The
  # hostname has to run up to a delimiter or the end of the string, so a URL
  # this cannot reduce exactly fails here rather than quietly deploying for a
  # truncated hostname.
  gateway_url = format(
    "https://%s",
    lower(regex(
      "^https?://(?:[^@/?#]*@)?([A-Za-z0-9](?:[-A-Za-z0-9]*[A-Za-z0-9])?(?:\\.[A-Za-z0-9](?:[-A-Za-z0-9]*[A-Za-z0-9])?)+)(?:[:/?#]|$)",
      p0_agentic_gateway_staged.example.domain_hosting.url,
    )[0]),
  )
}

# Uses the p0-security/p0-agentic-gateway-stack/kubernetes module:
# https://github.com/p0-security/terraform-kubernetes-p0-agentic-gateway-stack
module "agentic_gateway_stack" {
  source  = "p0-security/p0-agentic-gateway-stack/kubernetes"
  version = "1.0.0"

  release_name = p0_agentic_gateway_staged.example.id
  namespace    = p0_agentic_gateway_staged.example.kubernetes_namespace
  # The module requires https:// plus a bare lowercase hostname and rejects a
  # port, path or trailing slash at plan time. The resource does not, so reduce
  # its URL to a scheme and host rather than passing it through.
  gateway_url              = local.gateway_url
  lets_encrypt_email       = p0_agentic_gateway_staged.example.lets_encrypt_email
  oidc_client_id           = p0_agentic_gateway_staged.example.oidc_client_id
  storage_class            = p0_agentic_gateway_staged.example.storage_class
  p0_url                   = "https://api.p0.app/o/your-org"
  p0_audience              = "https://api.p0.app/o/your-org"
  p0_service_account_email = p0_agentic_gateway_staged.example.service_account_email
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
