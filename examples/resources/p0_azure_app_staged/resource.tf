locals {
  directory_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.directory_id
}

data "azuread_application_published_app_ids" "well_known" {}

data "azuread_service_principal" "msgraph" {
  client_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
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

resource "azuread_service_principal" "p0" {
  client_id  = azuread_application_registration.p0.client_id
  depends_on = [azuread_application_registration.p0]
}

resource "azuread_application_api_access" "msgraph_user_read_all" {
  application_id = azuread_application_registration.p0.id
  api_client_id  = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]

  role_ids = [
    data.azuread_service_principal.msgraph.app_role_ids["User.Read.All"],
  ]

  depends_on = [azuread_application_registration.p0]
}

resource "azuread_app_role_assignment" "msgraph_user_read_all_consent" {
  app_role_id         = data.azuread_service_principal.msgraph.app_role_ids["User.Read.All"]
  principal_object_id = azuread_service_principal.p0.object_id
  resource_object_id  = data.azuread_service_principal.msgraph.object_id

  depends_on = [
    azuread_service_principal.p0,
    azuread_application_api_access.msgraph_user_read_all,
  ]
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
  depends_on = [
    azuread_application_federated_identity_credential.p0,
    azuread_app_role_assignment.msgraph_user_read_all_consent,
  ]
  client_id = azuread_application_registration.p0.client_id
}
