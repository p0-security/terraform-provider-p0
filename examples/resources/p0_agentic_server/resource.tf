# See the p0_agentic_gateway example for the full staged-install pattern.
resource "p0_agentic_gateway_staged" "example" {
  id  = "primary"
  url = "https://gateway.example.com"
}

resource "helm_release" "oauthed_mcp" {
  name  = "oauthed-mcp"
  chart = "oci://registry-1.docker.io/p0security/p0-helm-oauthed-mcp"

  set = [{
    name  = "oauthed-mcp.mcpServer.manageAllowedEmails"
    value = p0_agentic_gateway_staged.example.service_account_email
  }]
}

resource "p0_agentic_gateway" "example" {
  id             = p0_agentic_gateway_staged.example.id
  oauth_endpoint = "https://oauth.gateway.example.com"
  depends_on     = [helm_release.oauthed_mcp]
}

resource "p0_agentic_identity_provider" "example" {
  id                   = "github-actions"
  issuer               = "https://token.actions.githubusercontent.com"
  dynamic_registration = true
}

# Requires the p0_aws_iam_write integration to already be installed for the same account.
resource "p0_aws_oidc_identity" "example" {
  id                = "github-actions"
  account_id        = "123456789012"
  oidc_provider_url = p0_agentic_identity_provider.example.issuer
  audience          = "https://github.com/my-org"
}

# An MCP server that federates AWS credentials via the identity above.
resource "p0_agentic_server" "aws_example" {
  id      = "aws-tools"
  gateway = p0_agentic_gateway.example.id
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
resource "p0_agentic_server" "custom_example" {
  id      = "custom-tools"
  gateway = p0_agentic_gateway.example.id
  credential = {
    type = "oauth"
    grant = {
      type      = "authorization_code"
      pkce      = true
      client_id = "my-oauth-client-id"
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
