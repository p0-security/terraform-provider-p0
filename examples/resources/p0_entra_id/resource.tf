locals {
  tenant_id = "12345678-1234-1234-1234-123456789012"

  # The Microsoft Graph application permissions P0's caller app needs to read
  # users, groups and directory roles.
  graph_permissions = [
    "Group.Read.All",
    "GroupMember.Read.All",
    "User.Read.All",
    "RoleManagement.Read.All",
    "Reports.Read.All",
    "AuditLog.Read.All",
    "Application.Read.All",
  ]
}

provider "azuread" {
  tenant_id = local.tenant_id
}

data "azuread_application_published_app_ids" "well_known" {}

data "azuread_service_principal" "msgraph" {
  client_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
}

resource "p0_entra_id_staged" "example" {
  tenant_id = local.tenant_id
}

resource "random_uuid" "user_impersonation" {}

# P0's caller app. P0 signs in as it to read the directory and to call the
# P0 Security Perimeter.
resource "azuread_application" "p0" {
  display_name     = "p0-security-azure-integration"
  sign_in_audience = "AzureADMyOrg"

  api {
    requested_access_token_version = 2

    oauth2_permission_scope {
      id                         = random_uuid.user_impersonation.result
      value                      = "user_impersonation"
      type                       = "User"
      enabled                    = true
      admin_consent_display_name = "User Impersonation"
      admin_consent_description  = "Allow the application to access the API on behalf of the signed-in user."
      user_consent_display_name  = "Access API"
      user_consent_description   = "Allow the application to access the API on your behalf."
    }
  }

  required_resource_access {
    resource_app_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]

    dynamic "resource_access" {
      for_each = local.graph_permissions
      content {
        id   = data.azuread_service_principal.msgraph.app_role_ids[resource_access.value]
        type = "Role"
      }
    }
  }
}

# Exposes the API as api://<client ID>.
resource "azuread_application_identifier_uri" "p0" {
  application_id = azuread_application.p0.id
  identifier_uri = "api://${azuread_application.p0.client_id}"
}

resource "azuread_service_principal" "p0" {
  client_id = azuread_application.p0.client_id
}

# Admin consent for each Graph permission.
resource "azuread_app_role_assignment" "graph" {
  for_each = toset(local.graph_permissions)

  app_role_id         = data.azuread_service_principal.msgraph.app_role_ids[each.value]
  principal_object_id = azuread_service_principal.p0.object_id
  resource_object_id  = data.azuread_service_principal.msgraph.object_id
}

# Lets P0 sign in as the app with its Google service account.
resource "azuread_application_federated_identity_credential" "p0" {
  application_id = azuread_application.p0.id
  display_name   = "P0Integration"
  description    = "P0 integration with Azure"
  issuer         = "https://accounts.google.com"
  audiences      = ["api://AzureADTokenExchange"]
  subject        = p0_entra_id_staged.example.service_account_id
}

resource "p0_entra_id" "example" {
  tenant_id          = p0_entra_id_staged.example.tenant_id
  sovereign_cloud_id = p0_entra_id_staged.example.sovereign_cloud_id
  client_id          = azuread_application.p0.client_id
  # userPrincipalName (the default), mail or otherMails.
  email_field = "userPrincipalName"

  depends_on = [
    azuread_application_federated_identity_credential.p0,
    azuread_app_role_assignment.graph,
  ]
}
