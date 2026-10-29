# See the p0_ai_gateway_staged example for the preceding steps
# (staging the gateway, and configuring it to trust the assigned service
# account) that this resource depends on.

resource "p0_ai_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url = "https://gateway.example.com"
  }
}

# Uses the p0-security/p0-ai-gateway-stack/kubernetes module:
# https://github.com/p0-security/terraform-kubernetes-p0-ai-gateway-stack
module "ai_gateway_stack" {
  source  = "p0-security/p0-ai-gateway-stack/kubernetes"
  version = "1.3.0"

  chart_version = "1.3.0-beta"
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

# Finalizes the install; depends_on ensures the gateway trusts P0's service
# account before verification is attempted.
#
# Add this resource only after the staged resource and the module are applied
# and the gateway is running, then apply again. braekhus_public_jwk is read
# from the running gateway, so Terraform cannot plan this resource, or anything
# else in the configuration, while the block is present without it.
resource "p0_ai_gateway" "example" {
  id = p0_ai_gateway_staged.example.id
  domain_hosting = {
    load_balancer_ip = "<your-gateway-loadbalancer-ip>"
  }
  log_project_id = "my-gcp-logging-project"
  # The contents of /p0-files/jwk.public.json in the braekhus-client pod, once it is running:
  #   kubectl exec -it deploy/braekhus-client -n <namespace> -c braekhus -- cat /p0-files/jwk.public.json | jq -Mr
  braekhus_public_jwk = "<public-jwk-json>"
  depends_on          = [module.ai_gateway_stack]
}
