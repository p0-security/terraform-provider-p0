# Completes the install of a connector that is staged and deployed. See gcp.tf
# in this directory for the full chain on Google Cloud Run.
resource "p0_jira_ai_connector" "example" {
  id = p0_jira_ai_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = p0_jira_ai_connector_staged.example.hosting.type
    project_id       = p0_jira_ai_connector_staged.example.hosting.project_id
    connector_name   = p0_jira_ai_connector_staged.example.hosting.connector_name
    connector_region = p0_jira_ai_connector_staged.example.hosting.connector_region
  }
}
