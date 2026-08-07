locals {
  # The Entra ID (Azure AD) tenant ID
  tenant_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.tenant_id
}

provider "azurerm" {
  features {}
}

data "azuread_application_published_app_ids" "well_known" {}

data "azuread_service_principal" "msgraph" {
  client_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
}

# ── Step 1: P0 Azure root integration ─────────────────────────────────────────
# Required before the iam-write component can be installed.
resource "p0_azure" "example" {
  directory_id = local.tenant_id
}

# ── Step 2: App Registration ───────────────────────────────────────────────────
# See examples/resources/p0_entra_app/ for the full app registration setup
# (Graph permissions, federated credential, and the p0_entra_app resource
# itself). This example assumes that resource already exists, and layers on
# the additional setup that iam_write requires.
resource "azuread_application_registration" "p0" {
  display_name = "p0-security-entra-integration"
}

# Expose an API with the user_impersonation scope so the Function App can
# accept tokens issued for this application.
resource "azuread_application" "p0_api" {
  display_name = azuread_application_registration.p0.display_name
  client_id    = azuread_application_registration.p0.client_id

  api {
    requested_access_token_version = 2

    oauth2_permission_scope {
      admin_consent_description  = "Allow the application to access the API on behalf of the signed-in user."
      admin_consent_display_name = "User Impersonation"
      id                         = "00000000-0000-0000-0000-000000000001"
      enabled                    = true
      type                       = "User"
      value                      = "user_impersonation"
    }
  }
}

# Federated credential — lets P0's service account obtain Azure tokens
# without a client secret.
resource "azuread_application_federated_identity_credential" "p0" {
  depends_on     = [p0_azure.example, azuread_application_registration.p0]
  application_id = azuread_application_registration.p0.id
  display_name   = "P0Integration"
  description    = "P0 service account credential"
  issuer         = p0_azure.example.credential_info.issuer
  subject        = p0_azure.example.service_account_id
  audiences      = p0_azure.example.credential_info.audiences
}

resource "azuread_service_principal" "p0" {
  client_id = azuread_application_registration.p0.client_id
}

# Register the App Registration with P0. See examples/resources/p0_entra_app/
# for the full recommended Graph permission set.
resource "p0_entra_app" "example" {
  depends_on = [
    azuread_application_federated_identity_credential.p0,
  ]
  client_id = azuread_application_registration.p0.client_id
}

# ── Step 3: Function App (P0 Security Perimeter) ───────────────────────────────
resource "azurerm_resource_group" "p0" {
  name     = "p0-security-perimeter-rg"
  location = "West US 2"
}

resource "azurerm_storage_account" "p0" {
  name                     = "p0securityperimeter"
  resource_group_name      = azurerm_resource_group.p0.name
  location                 = azurerm_resource_group.p0.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_service_plan" "p0" {
  name                = "p0-security-perimeter-plan"
  location            = azurerm_resource_group.p0.location
  resource_group_name = azurerm_resource_group.p0.name
  os_type             = "Linux"
  sku_name            = "EP1"
}

resource "azurerm_linux_function_app" "p0" {
  name                       = "p0-security-perimeter-${replace(local.tenant_id, "-", "")}"
  location                   = azurerm_resource_group.p0.location
  resource_group_name        = azurerm_resource_group.p0.name
  service_plan_id            = azurerm_service_plan.p0.id
  storage_account_name       = azurerm_storage_account.p0.name
  storage_account_access_key = azurerm_storage_account.p0.primary_access_key
  https_only                 = true

  identity {
    type = "SystemAssigned"
  }

  site_config {
    application_stack {
      docker {
        registry_url = "https://docker.io"
        image_name   = "p0security/p0-security-perimeter-entra"
        image_tag    = "latest"
      }
    }
  }

  app_settings = {
    MANAGED_IDENTITY_ID = azurerm_linux_function_app.p0.identity[0].principal_id
    CALLER_APP_ID       = azuread_application_registration.p0.client_id
  }

  auth_settings_v2 {
    auth_enabled           = true
    unauthenticated_action = "Return401"

    active_directory_v2 {
      client_id            = azuread_application_registration.p0.client_id
      tenant_auth_endpoint = "https://login.microsoftonline.com/${local.tenant_id}/v2.0"
      allowed_audiences    = [azuread_application_registration.p0.client_id]
    }

    login {}
  }
}

# Grant the Microsoft Graph permissions required by the Function App's
# system-assigned managed identity to read/write role assignments and group
# memberships in the directory. Note these are granted to the Function App's
# managed identity, not to the shared App Registration's service principal
# (which only ever receives the read-only permissions in
# examples/resources/p0_entra_app/).
locals {
  function_app_msgraph_permissions = [
    "User.Read.All",
    "RoleManagement.ReadWrite.Directory",
    "GroupMember.Read.All",
    "GroupMember.ReadWrite.All",
    "Group.Read.All",
  ]
}

resource "azuread_app_role_assignment" "function_app_msgraph" {
  for_each = toset(local.function_app_msgraph_permissions)

  app_role_id         = data.azuread_service_principal.msgraph.app_role_ids[each.value]
  principal_object_id = azurerm_linux_function_app.p0.identity[0].principal_id
  resource_object_id  = data.azuread_service_principal.msgraph.object_id

  depends_on = [azurerm_linux_function_app.p0]
}

# ── Step 4: P0 Entra ID IAM Write ─────────────────────────────────────────────
resource "p0_entra_id_iam_write" "example" {
  depends_on = [
    p0_azure.example,
    p0_entra_app.example,
    azurerm_linux_function_app.p0,
    azuread_app_role_assignment.function_app_msgraph,
  ]

  tenant_id          = local.tenant_id
  client_id          = azuread_application_registration.p0.client_id
  sovereign_cloud_id = "AzureCloud"
  email_field        = "userPrincipalName"
}
