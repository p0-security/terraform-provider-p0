# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_tableau_ai_staged" "example" {
  id          = "my-tableau-site"
  server      = "https://us-east-1.online.tableau.com"
  content_url = "acme"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# See the p0_tableau_ai example for the next steps (deploying the connector
# and its admin connected app's secret) that complete the install.
