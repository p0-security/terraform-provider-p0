# See the p0_ai_gateway example for the full staged-install pattern.
resource "p0_ai_gateway_staged" "example" {
  id = "primary"
  domain_hosting = {
    url            = "https://gateway.example.com"
    oauth_endpoint = "https://oauth.gateway.example.com"
  }
}

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

resource "p0_ai_gateway" "example" {
  id = p0_ai_gateway_staged.example.id
  domain_hosting = {
    load_balancer_ip = "<your-gateway-loadbalancer-ip>"
  }
  depends_on = [module.ai_gateway_stack]
}

resource "p0_identity_provider" "example" {
  id                   = "github-actions"
  issuer               = "https://token.actions.githubusercontent.com"
  dynamic_registration = true
}

# See the p0_aws_oidc_identity example for the full staged-install pattern
# (creating the AWS-side OIDC provider and role). Requires the
# p0_aws_iam_write integration to already be installed for the same account.
resource "p0_aws_oidc_identity_staged" "example" {
  id                = "github-actions"
  account_id        = "123456789012"
  oidc_provider_url = p0_identity_provider.example.issuer
  audience          = "https://github.com/my-org"
}

resource "p0_aws_oidc_identity" "example" {
  id                = p0_aws_oidc_identity_staged.example.id
  account_id        = p0_aws_oidc_identity_staged.example.account_id
  oidc_provider_url = p0_aws_oidc_identity_staged.example.oidc_provider_url
  audience          = p0_aws_oidc_identity_staged.example.audience
}

# An MCP server that federates AWS credentials via the identity above.
resource "p0_ai_gateway_server" "aws_example" {
  id      = "aws-tools"
  gateway = p0_ai_gateway.example.id
  credential = {
    type     = "aws"
    provider = p0_aws_oidc_identity.example.id
  }
  definition = {
    type = "p0"
    id   = "aws"
  }
}

# A custom, externally hosted MCP server that authenticates end users via OAuth.
resource "p0_ai_gateway_server" "custom_example" {
  id      = "custom-tools"
  gateway = p0_ai_gateway.example.id
  credential = {
    type = "oauth"
    grant = {
      type = "authorization_code"
    }
  }
  definition = {
    type = "custom"
    hosting = {
      type  = "external"
      url   = "https://tools.example.com/mcp"
      label = "Example tools"
    }
    prompt = "Use these tools to look up example data."
  }
}

# A predefined server whose credential P0 stores in a secret manager. Install the
# matching integration (here p0_github_ai) first. Each grant names the secret.
resource "p0_ai_gateway_server" "github_example" {
  id         = "github"
  gateway    = p0_ai_gateway.example.id
  credential = { type = "vault" }
  definition = {
    type = "p0"
    id   = "github"
  }
}

# A predefined server whose URL takes an instance, here a Databricks workspace's
# deployment name. Install the matching integration (here p0_databricks_ai_account)
# first. Changing the instance replaces the server.
resource "p0_ai_gateway_server" "databricks_example" {
  id         = "databricks-genie"
  gateway    = p0_ai_gateway.example.id
  credential = { type = "generic" }
  definition = {
    type     = "p0"
    id       = "databricksGenie"
    instance = "dbc-a1b2c3d4-e5f6"
  }
}
