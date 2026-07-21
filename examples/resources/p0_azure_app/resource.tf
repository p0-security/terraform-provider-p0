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

# Stage the P0 app registration to obtain the app name and federated-credential
# parameters. See examples/resources/p0_azure_app_staged/ for the full
# explanation of the staged outputs.
resource "p0_azure_app_staged" "example" {
  depends_on = [p0_azure.example]
}

# Create the Azure AD application from the staged app name.
resource "azuread_application_registration" "p0" {
  display_name = p0_azure_app_staged.example.app_name
}

# P0's app installer requires the application's service principal to exist in the
# tenant: completing p0_azure_app acquires a token as the app and looks the
# principal up by app ID, so the install fails if the service principal is absent.
resource "azuread_service_principal" "p0" {
  client_id  = azuread_application_registration.p0.client_id
  depends_on = [azuread_application_registration.p0]
}

# Grant and admin-consent the Microsoft Graph User.Read.All permission P0 uses to
# resolve access-request principals to Entra ID users (verified later by
# p0_azure_iam_write).
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

# Create the federated identity credential so P0 can authenticate as the app via
# workload identity federation, using p0_azure.service_account_id as the subject.
resource "azuread_application_federated_identity_credential" "p0" {
  depends_on     = [p0_azure_app_staged.example, azuread_application_registration.p0]
  application_id = azuread_application_registration.p0.id
  display_name   = p0_azure_app_staged.example.credential_info.name
  description    = p0_azure_app_staged.example.credential_info.description
  issuer         = p0_azure_app_staged.example.credential_info.issuer
  audiences      = p0_azure_app_staged.example.credential_info.audiences
  subject        = p0_azure.example.service_account_id
}

# Complete the app install by pointing client_id at the new application.
resource "p0_azure_app" "example" {
  depends_on = [
    azuread_application_federated_identity_credential.p0,
    azuread_service_principal.p0,
    azuread_app_role_assignment.msgraph_user_read_all_consent,
  ]
  client_id = azuread_application_registration.p0.client_id
}
