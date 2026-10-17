# Requires the p0_gcp integration to already be installed for the same
# organization.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

resource "p0_looker_ai_staged" "example" {
  id           = "acme"
  instance_url = "https://acme.looker.app"
  # The Google Secret Manager secret that will hold the Looker Admin key.
  admin_key_secret = "p0_install_looker_acme_admin-key"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "my-project-id"
  }

  depends_on = [p0_gcp.example]
}

# The p0_looker_ai example shows the rest of the install: deploying the
# connector, adding the Admin key to its secret, and creating p0_looker_ai.
