# Must pre-exist in the P0 app: the Okta group (via an installed Okta directory
# listing integration, see p0_okta_directory_listing), the "aws" integration (see
# p0_aws_iam_write), and the PagerDuty integration (connected in-app, not via Terraform).
resource "p0_access_policy" "example" {
  name = "okta-aws-developers-oncall"
  requestor = {
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "okta"
      id        = "00abcdefghijklmno697"
      label     = "AWS Developers"
    }]
  }
  resource = {
    type    = "integration"
    service = "aws"
    filters = {
      "tag" = {
        effect  = "keep"
        key     = "p0_grantable"
        pattern = "1|true"
      }
    }
  }
  approval = [{
    type        = "auto"
    integration = "pagerduty"
    options = {
      require_reason = true
    }
  }]
}

# Agentic requestor: matches agent sessions rather than a human directly.
# This example matches only the "my-mcp-client" MCP client agent, as long as
# no human user is present (a headless agent session), and denies access.
resource "p0_access_policy" "agentic_headless" {
  name = "mcp-agent-headless"
  requestor = {
    type = "agentic"
    agent = {
      type      = "mcp-client"
      client_id = "my-mcp-client"
    }
    user = {
      type = "none"
    }
  }
  resource = {
    type    = "integration"
    service = "aws"
    filters = {
      "tag" = {
        effect  = "keep"
        key     = "p0_grantable"
        pattern = "1|true"
      }
    }
  }
  approval = [{
    type = "deny"
  }]
}

# Agentic requestor: matches an agent federated by an identity provider,
# owned by a member of a directory group.
resource "p0_access_policy" "agentic_federated" {
  name = "federated-agent-owner-group"
  requestor = {
    type = "agentic"
    agent = {
      type            = "provider"
      provider_id     = "okta-idp"
      subject_pattern = "^svc-.*$"
    }
    user = {
      type   = "group"
      effect = "keep"
      groups = [{
        directory = "okta"
        id        = "00abcdefghijklmno697"
        label     = "AWS Developers"
      }]
    }
  }
  resource = {
    type    = "integration"
    service = "aws"
  }
  approval = [{
    type        = "auto"
    integration = "pagerduty"
    options = {
      require_reason = true
    }
  }]
}
