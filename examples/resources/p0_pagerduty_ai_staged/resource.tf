# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_pagerduty_ai_staged" "example" {
  id        = "my-pagerduty-account"
  region    = "us"
  subdomain = "acme"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# See the p0_pagerduty_ai example for the next steps (deploying the connector
# and its client secret's secret) that complete the install.
