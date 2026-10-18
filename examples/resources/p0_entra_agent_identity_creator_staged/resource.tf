# Staging picks the P0 service account that calls the connector. Its
# service_account_id is the subject of the federated identity credential on the
# connector's app registration. See the p0_entra_agent_identity_creator example
# for the full chain.
resource "p0_entra_agent_identity_creator_staged" "example" {
  directory_id    = "11111111-1111-1111-1111-111111111111"
  subscription_id = "22222222-2222-2222-2222-222222222222"
  region          = "eastus"
  # Also the connector's P0_APP_ROLE_ALLOWLIST: keep the two the same.
  app_role_allowlist = "Sites.Selected"
}
