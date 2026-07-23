locals {
  directory_id    = "12345678-1234-1234-1234-123456789012"
  subscription_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.directory_id
}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

data "azuread_application_published_app_ids" "well_known" {}

data "azuread_service_principal" "msgraph" {
  client_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
}

resource "p0_azure" "example" {
  directory_id = local.directory_id
}

# P0 app registration chain: stage for the app name + federated-credential
# params, create the app and its federated credential, then complete via
# p0_azure_app.
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

# User.Read.All Microsoft Graph permission, verified at the IAM-write install;
# resolves access-request principals to Entra ID users.
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

# Stage to obtain the custom role P0 needs to manage role assignments in the subscription.
resource "p0_azure_iam_write_staged" "example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.subscription_id
}

# Create the custom role from the staged spec and assign it to P0's service principal.
resource "azurerm_role_definition" "p0_iam_management" {
  name              = p0_azure_iam_write_staged.example.custom_role.name
  description       = p0_azure_iam_write_staged.example.custom_role.description
  scope             = p0_azure_iam_write_staged.example.custom_role.assignable_scope
  assignable_scopes = [p0_azure_iam_write_staged.example.custom_role.assignable_scope]

  permissions {
    actions = p0_azure_iam_write_staged.example.custom_role.actions
  }
}

resource "azurerm_role_assignment" "p0_iam_management" {
  scope              = p0_azure_iam_write_staged.example.custom_role.assignable_scope
  role_definition_id = azurerm_role_definition.p0_iam_management.role_definition_resource_id
  principal_id       = azuread_service_principal.p0.object_id

  # SP was just created and may not have propagated to Azure AD yet.
  skip_service_principal_aad_check = true

  # ABAC condition blocking P0 from assigning/revoking roles for its own service
  # principal (privilege escalation); P0 always returns it in the staged custom_role.
  condition         = p0_azure_iam_write_staged.example.custom_role.condition
  condition_version = "2.0"
}

# Complete the IAM-write install once the role assignment exists.
resource "p0_azure_iam_write" "example" {
  depends_on      = [azurerm_role_assignment.p0_iam_management]
  subscription_id = local.subscription_id
}
