# Installs P0's SharePoint connector on Azure Container Apps, beside the Entra
# Agent Identity Creator, in the same configuration. Full chain:
# p0_entra_agent_identity_creator (see its example, which declares the
# providers) -> p0_sharepoint_connector_staged -> the connector's Container
# App, managed identity and app registration -> p0_sharepoint_connector.
#
# Whoever applies this needs two sets of roles. In Microsoft Entra ID:
# Application Developer (or a directory that lets users register
# applications) and Privileged Role Administrator. On the subscription:
# Contributor plus Role Based Access Control Administrator, or Owner.

resource "p0_sharepoint_connector_staged" "example" {
  directory_id = p0_entra_agent_identity_creator.example.directory_id
}

locals {
  sharepoint = {
    directory_id = p0_sharepoint_connector_staged.example.directory_id
    # P0 reads the connector's Container App at this resource ID, in the Entra
    # Agent Identity Creator's resource group.
    resource_group_name = split("/", p0_sharepoint_connector_staged.example.container_app_resource_id)[4]
    connector_name      = p0_sharepoint_connector_staged.example.connector_name
    location            = p0_entra_agent_identity_creator_staged.example.region
  }
}

# The connector's own identity, which holds the Microsoft Graph app roles
# below. P0 calls the connector through the app registration further down
# instead.
resource "azurerm_user_assigned_identity" "p0_sharepoint_ai_connector" {
  name                = local.sharepoint.connector_name
  location            = local.sharepoint.location
  resource_group_name = local.sharepoint.resource_group_name
}

data "azuread_service_principal" "p0_sharepoint_ai_msgraph" {
  client_id = "00000003-0000-0000-c000-000000000000"
}

# Sites.FullControl.All can change the permissions of any site in the
# directory. The connector writes them only for apps the Entra Agent Identity
# Creator owns, which Application.Read.All lets it check.
resource "azuread_app_role_assignment" "p0_sharepoint_ai_connector_graph" {
  for_each = toset([
    "Application.Read.All",
    "Sites.FullControl.All",
  ])

  app_role_id         = data.azuread_service_principal.p0_sharepoint_ai_msgraph.app_role_ids[each.value]
  principal_object_id = azurerm_user_assigned_identity.p0_sharepoint_ai_connector.principal_id
  resource_object_id  = data.azuread_service_principal.p0_sharepoint_ai_msgraph.object_id
}

# Runs in the Entra Agent Identity Creator's Container Apps environment.
resource "azurerm_container_app" "p0_sharepoint_ai_connector" {
  name                         = local.sharepoint.connector_name
  container_app_environment_id = p0_entra_agent_identity_creator.example.environment_id
  resource_group_name          = local.sharepoint.resource_group_name
  revision_mode                = "Single"

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.p0_sharepoint_ai_connector.id]
  }

  ingress {
    external_enabled = true
    target_port      = 8080
    traffic_weight {
      latest_revision = true
      percentage      = 100
    }
  }

  template {
    min_replicas = 1
    max_replicas = 1

    container {
      name = "connector"
      # P0's connector image, pinned by its tag and digest.
      # TODO(INT-780): the published tag and digest, from the app's
      # CONNECTOR_IMAGE once INT-779 pins it.
      image  = "docker.io/p0security/p0-connector-sharepoint-ai-azure:sha-0000000@sha256:0000000000000000000000000000000000000000000000000000000000000000"
      cpu    = 0.25
      memory = "0.5Gi"

      env {
        name  = "AZURE_CLIENT_ID"
        value = azurerm_user_assigned_identity.p0_sharepoint_ai_connector.client_id
      }

      env {
        name  = "P0_INVOKER_TENANT_ID"
        value = local.sharepoint.directory_id
      }

      env {
        name  = "P0_INVOKER_CLIENT_ID"
        value = azuread_application.p0_sharepoint_ai_connector.client_id
      }

      # The connector grants only apps this identity owns: the ones the Entra
      # Agent Identity Creator made for a grant.
      env {
        name  = "P0_FORGE_CLIENT_ID"
        value = p0_entra_agent_identity_creator.example.forge_client_id
      }
    }
  }
}

data "azuread_client_config" "p0_sharepoint_ai_connector" {}

# The app registration P0 calls the connector as.
resource "azuread_application" "p0_sharepoint_ai_connector" {
  display_name     = "P0 SharePoint connector"
  sign_in_audience = "AzureADMyOrg"
  owners           = [data.azuread_client_config.p0_sharepoint_ai_connector.object_id]

  api {
    requested_access_token_version = 2
  }

  lifecycle {
    ignore_changes = [identifier_uris]
  }
}

resource "azuread_application_identifier_uri" "p0_sharepoint_ai_connector" {
  application_id = azuread_application.p0_sharepoint_ai_connector.id
  identifier_uri = "api://${azuread_application.p0_sharepoint_ai_connector.client_id}"
}

resource "azuread_service_principal" "p0_sharepoint_ai_connector" {
  client_id = azuread_application.p0_sharepoint_ai_connector.client_id
  owners    = [data.azuread_client_config.p0_sharepoint_ai_connector.object_id]
}

# P0 signs in as the app registration with a token for the Entra Agent
# Identity Creator's service account instead of a secret. The SharePoint
# connector has no service account of its own.
resource "azuread_application_federated_identity_credential" "p0_sharepoint_ai_invoker" {
  application_id = azuread_application.p0_sharepoint_ai_connector.id
  display_name   = "p0-invoker"
  issuer         = "https://accounts.google.com"
  subject        = p0_entra_agent_identity_creator_staged.example.service_account_id
  audiences      = ["api://AzureADTokenExchange"]
}

# P0 reads the Container App, as the app registration, to get the connector's
# URL and check which identity it trusts.
resource "azurerm_role_assignment" "p0_sharepoint_ai_reader" {
  scope                = azurerm_container_app.p0_sharepoint_ai_connector.id
  role_definition_name = "Reader"
  principal_id         = azuread_service_principal.p0_sharepoint_ai_connector.object_id
  principal_type       = "ServicePrincipal"
}

# Easy Auth admits only tokens from this directory for the connector's app
# registration.
resource "azapi_resource" "p0_sharepoint_ai_connector_auth" {
  type      = "Microsoft.App/containerApps/authConfigs@2024-03-01"
  name      = "current"
  parent_id = azurerm_container_app.p0_sharepoint_ai_connector.id

  body = {
    properties = {
      platform = {
        enabled = true
      }
      globalValidation = {
        unauthenticatedClientAction = "Return401"
      }
      identityProviders = {
        azureActiveDirectory = {
          enabled = true
          registration = {
            clientId     = azuread_application.p0_sharepoint_ai_connector.client_id
            openIdIssuer = "https://login.microsoftonline.com/${local.sharepoint.directory_id}/v2.0"
          }
          validation = {
            allowedAudiences = [azuread_application.p0_sharepoint_ai_connector.client_id]
            defaultAuthorizationPolicy = {
              allowedApplications = [azuread_application.p0_sharepoint_ai_connector.client_id]
            }
          }
        }
      }
    }
  }
}

# Completes the install. Creating it reads the Container App and checks that P0
# can reach the connector, which can take a few minutes while the federated
# identity credential starts working.
resource "p0_sharepoint_connector" "example" {
  directory_id        = p0_sharepoint_connector_staged.example.directory_id
  connector_client_id = azuread_application.p0_sharepoint_ai_connector.client_id

  depends_on = [
    azuread_application_federated_identity_credential.p0_sharepoint_ai_invoker,
    azuread_application_identifier_uri.p0_sharepoint_ai_connector,
    azurerm_role_assignment.p0_sharepoint_ai_reader,
    azapi_resource.p0_sharepoint_ai_connector_auth,
    azuread_app_role_assignment.p0_sharepoint_ai_connector_graph,
  ]
}
