locals {
  directory_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.directory_id
}

resource "p0_azure" "example" {
  directory_id = local.directory_id
}

resource "p0_azure_app_staged" "example" {
  depends_on = [p0_azure.example]
}

resource "azuread_application_registration" "p0" {
  display_name = p0_azure_app_staged.example.app_name
}

resource "azuread_application_federated_identity_credential" "p0" {
  depends_on     = [p0_azure.example, p0_azure_app_staged.example, azuread_application_registration.p0]
  application_id = azuread_application_registration.p0.id
  display_name   = p0_azure_app_staged.example.credential_info.name
  description    = p0_azure_app_staged.example.credential_info.description
  issuer         = p0_azure_app_staged.example.credential_info.issuer
  audiences      = p0_azure_app_staged.example.credential_info.audiences
  subject        = p0_azure.example.service_account_id
}

resource "p0_azure_app" "example" {
  depends_on = [azuread_application_federated_identity_credential.p0]
  client_id  = azuread_application_registration.p0.client_id
}
