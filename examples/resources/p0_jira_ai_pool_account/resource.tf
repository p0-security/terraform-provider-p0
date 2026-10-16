# Adds an Atlassian service account that P0 leases to agent sessions on a
# site. P0 names the secret its OAuth client secret goes in, as secret_name.
# See gcp.tf in this directory for creating that secret.
resource "p0_jira_ai_pool_account" "example" {
  id        = "agent-1"
  site      = p0_jira_ai_site.example.id
  email     = "p0-agent-1@serviceaccount.atlassian.com"
  client_id = "ZyXwVuTsRqPoNmLkJiHgFeDcBa543210"
}
