locals {
  # The Entra ID (Azure AD) tenant ID
  tenant_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.tenant_id
}

data "azuread_application_published_app_ids" "well_known" {}

data "azuread_service_principal" "msgraph" {
  client_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
}

# ── Step 1: P0 Azure root integration ─────────────────────────────────────────
# Required before the entra-app component can be installed.
resource "p0_azure" "example" {
  directory_id = local.tenant_id
}

# ── Step 2: App Registration ───────────────────────────────────────────────────
# This single App Registration is shared by p0_entra_id_iam_write and
# p0_entra_id_iam_assessment; each resource layers on the additional Graph
# permissions and, for iam_write, the Function App it needs.
resource "azuread_application_registration" "p0" {
  display_name = "p0-security-entra-integration"
}

resource "azuread_service_principal" "p0" {
  client_id  = azuread_application_registration.p0.client_id
  depends_on = [azuread_application_registration.p0]
}

# Grant the Microsoft Graph RoleManagement.Read.Directory permission, required
# to read Entra ID directory role assignments.
resource "azuread_application_api_access" "msgraph_role_management_read" {
  application_id = azuread_application_registration.p0.id
  api_client_id  = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]

  role_ids = [
    data.azuread_service_principal.msgraph.app_role_ids["RoleManagement.Read.Directory"],
  ]

  depends_on = [azuread_application_registration.p0]
}

resource "azuread_app_role_assignment" "msgraph_role_management_read_consent" {
  app_role_id         = data.azuread_service_principal.msgraph.app_role_ids["RoleManagement.Read.Directory"]
  principal_object_id = azuread_service_principal.p0.object_id
  resource_object_id  = data.azuread_service_principal.msgraph.object_id

  depends_on = [
    azuread_service_principal.p0,
    azuread_application_api_access.msgraph_role_management_read,
  ]
}

# Federated credential — lets P0's service account obtain Azure tokens without
# a client secret.
resource "azuread_application_federated_identity_credential" "p0" {
  depends_on     = [p0_azure.example, azuread_application_registration.p0]
  application_id = azuread_application_registration.p0.id
  display_name   = "P0Integration"
  description    = "P0 service account credential"
  issuer         = p0_azure.example.credential_info.issuer
  subject        = p0_azure.example.service_account_id
  audiences      = p0_azure.example.credential_info.audiences
}

# ── Step 3: P0 Entra App Registration ─────────────────────────────────────────
resource "p0_entra_app" "example" {
  depends_on = [
    azuread_application_federated_identity_credential.p0,
    azuread_app_role_assignment.msgraph_role_management_read_consent,
  ]
  client_id = azuread_application_registration.p0.client_id
}
