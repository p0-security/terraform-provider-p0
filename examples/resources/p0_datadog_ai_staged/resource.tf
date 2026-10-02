# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_datadog_ai_staged" "example" {
  id   = "my-datadog-org"
  site = "us5"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# See the p0_datadog_ai example for the next steps (deploying the connector and
# its admin keys secret) that complete the install.
