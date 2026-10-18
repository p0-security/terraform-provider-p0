# Staging names the connector's Container App. P0 calls the connector as the
# Entra Agent Identity Creator's service account, so this requires
# p0_entra_agent_identity_creator on the same directory. See the
# p0_sharepoint_connector example for the full chain.
resource "p0_sharepoint_connector_staged" "example" {
  directory_id = p0_entra_agent_identity_creator.example.directory_id
}
