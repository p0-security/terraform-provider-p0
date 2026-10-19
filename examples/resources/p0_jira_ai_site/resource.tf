# Adds a Jira Cloud site, reached through an installed connector. The admin
# client's secret must already be in Google Secret Manager, in the vault's
# project. See gcp.tf in this directory for the grants the connector needs.
resource "p0_jira_ai_site" "example" {
  id        = "acme"
  connector = p0_jira_ai_connector.example.id
  site_url  = "https://acme.atlassian.net"

  vault = {
    type       = "gcp-sm"
    project_id = "my-secrets-project-id"
  }

  admin_client_id   = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
  admin_secret_name = "jira-acme-admin-client-secret"
}
