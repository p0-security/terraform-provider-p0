# Root of every Azure integration: registers your Microsoft Entra ID (Azure AD)
# tenant with P0 and is a prerequisite for all other Azure resources. On its own
# it only records the directory ID; the integration is not functional until the
# P0 Entra app registration and its federated identity credential exist. See
# examples/resources/p0_azure_app/ for that full chain.
resource "p0_azure" "example" {
  # Your Microsoft Entra ID (Azure AD) tenant / directory ID (a UUID).
  directory_id = "12345678-1234-1234-1234-123456789012"
}
