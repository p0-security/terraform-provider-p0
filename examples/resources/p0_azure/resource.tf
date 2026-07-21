# p0_azure is the root of every Azure integration: it registers your Microsoft
# Entra ID (Azure AD) tenant with P0. It is a prerequisite for all other Azure
# resources (p0_azure_app, p0_azure_iam_write, p0_azure_bastion_host,
# p0_ssh_azure, ...), which depend on this resource.
#
# On its own this install only records the directory ID; the integration is not
# functional until the P0 Entra app registration and its federated identity
# credential exist. Those are created by staging the app with
# p0_azure_app_staged, provisioning the azuread_application_registration +
# azuread_application_federated_identity_credential from its computed outputs,
# and completing the install with p0_azure_app. See
# examples/resources/p0_azure_app/ for that full chain.
resource "p0_azure" "example" {
  # Your Microsoft Entra ID (Azure AD) tenant / directory ID (a UUID).
  directory_id = "12345678-1234-1234-1234-123456789012"
}
