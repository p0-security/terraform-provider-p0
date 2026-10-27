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
  version = "1.3.0"

  chart_version = "1.3.0-beta"
  release_name  = p0_ai_gateway_staged.example.id
  namespace     = "p0-ai-gateway"

  values = [
    yamlencode({
      letsEncrypt = {
        env   = "prod"
        email = "admin@example.com"
      }
      postgresql   = { storageClass = "gp2" }
      victorialogs = { storageClass = "gp2" }
      valkey       = { storageClass = "<encrypted-storage-class>" }
      "ai-gateway" = {
        gateway = {
          className = p0_ai_gateway_staged.example.id
          host      = "gateway.example.com"
        }
        authServer = {
          oidcClientId = "your-upstream-oidc-client-id"
          gatewayIss   = "https://gateway.example.com"
        }
        accessProxy = {
          gatewayIss          = "https://gateway.example.com"
          manageAllowedEmails = p0_ai_gateway_staged.example.service_account_email
        }
      }
    }),
  ]
}
