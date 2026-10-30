# P0 assigns a service account to communicate with your gateway.
resource "p0_ai_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
}

# Your gateway must trust that service account before P0 can finish
# installing (see the p0_ai_gateway example for the next step).
# Uses the p0-security/p0-ai-gateway-stack/kubernetes module:
# https://github.com/p0-security/terraform-kubernetes-p0-ai-gateway-stack
module "ai_gateway_stack" {
  source  = "p0-security/p0-ai-gateway-stack/kubernetes"
  version = "1.4.0"

  chart_version = "1.4.0-beta"
  namespace     = "p0-ai-gateway"

  values = [
    yamlencode({
      letsEncrypt = {
        env   = "prod"
        email = "admin@example.com"
      }
      # local-path is for non-production only; its data is not durable.
      # Valkey holds credentials, so production needs an encrypted storage class.
      postgresql   = { storageClass = "local-path" }
      victorialogs = { storageClass = "local-path" }
      valkey       = { storageClass = "local-path" }
      "ai-gateway" = {
        gateway = {
          className = "primary"
          host      = "gateway.example.com"
        }
        authServer = {
          oidcClientId = "your-upstream-oidc-client-id"
          p0Url        = "https://api.p0.app/o/<your-org>"
        }
        accessProxy = {
          p0Url               = "https://api.p0.app/o/<your-org>"
          manageAllowedEmails = p0_ai_gateway_staged.example.service_account_email
        }
        braekhus = {
          orgSlug   = "<your-org>"
          gatewayId = p0_ai_gateway_staged.example.id
        }
      }
    }),
  ]
}
