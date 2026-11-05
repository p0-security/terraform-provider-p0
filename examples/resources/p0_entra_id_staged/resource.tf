resource "p0_entra_id_staged" "example" {
  # Your Microsoft Entra ID tenant (directory) ID.
  tenant_id = "12345678-1234-1234-1234-123456789012"
  # AzureCloud (the default) or AzureUSGovernment.
  sovereign_cloud_id = "AzureCloud"
}

# The p0_entra_id example shows the rest of the install: the caller app
# registration, its federated credential and Graph permissions, and
# p0_entra_id.
