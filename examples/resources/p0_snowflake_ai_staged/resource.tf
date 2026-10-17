# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_snowflake_ai_staged" "example" {
  # The Snowflake account identifier, in its ORGNAME-ACCOUNTNAME form.
  id = "MYORG-MYACCOUNT"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# See the p0_snowflake_ai example for the next steps (deploying the connector and
# its private key secret) that complete the install.
