locals {
  # p0_app_client_id is the P0 root app registration (see examples/resources/p0_azure_app/),
  # NOT the session-terminator app created below. subscription_id is where the jump
  # hosts and Function App connector live.
  directory_id     = "11111111-1111-1111-1111-111111111111"
  subscription_id  = "22222222-2222-2222-2222-222222222222"
  p0_app_client_id = "33333333-3333-3333-3333-333333333333"
  location         = "eastus"
}

provider "azuread" {
  tenant_id = local.directory_id
}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

# --- P0 root install (see examples/resources/p0_azure/ and p0_azure_app/) ---
resource "p0_azure" "example" {
  directory_id = local.directory_id
}

# Root app P0 authenticates as for the tenant (distinct from the session-terminator
# app below); create it per examples/resources/p0_azure_app/.
resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = local.p0_app_client_id
}

# --- Session-terminator app registration ---
# P0 authenticates as this app (via workload identity federation) when it calls the
# Function App to dispatch privileged commands to your jump hosts.
resource "azuread_application_registration" "session_terminator" {
  display_name = "P0 Session Terminator"
  # v2 access tokens, matching the tenant_auth_endpoint used by Easy Auth below.
  requested_access_token_version = 2
}

resource "azuread_service_principal" "session_terminator" {
  client_id = azuread_application_registration.session_terminator.client_id
}

# api:// identifier URI so the app can be the audience of tokens Easy Auth validates.
resource "azuread_application_identifier_uri" "session_terminator" {
  application_id = azuread_application_registration.session_terminator.id
  identifier_uri = "api://${azuread_application_registration.session_terminator.client_id}"
}

# --- Function App connector ---
# Runs P0's session-terminator package, which uses Azure Run Command to terminate
# live SSH sessions on your jump host VMs. Needs an Elastic Premium (EP1) Linux plan.
resource "azurerm_resource_group" "jump_host_connector" {
  name     = "p0-jump-host-connector"
  location = local.location
}

# Name must be globally unique, 3-24 lowercase alphanumerics; change before applying.
resource "azurerm_storage_account" "jump_host_connector" {
  name                     = "p0jumphostconn"
  resource_group_name      = azurerm_resource_group.jump_host_connector.name
  location                 = azurerm_resource_group.jump_host_connector.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_service_plan" "jump_host_connector" {
  name                = "p0-jump-host-connector-plan"
  resource_group_name = azurerm_resource_group.jump_host_connector.name
  location            = azurerm_resource_group.jump_host_connector.location
  os_type             = "Linux"
  sku_name            = "EP1"
}

# Function App name must be globally unique; change this before applying.
resource "azurerm_linux_function_app" "jump_host_connector" {
  name                       = "p0-jump-host-connector"
  resource_group_name        = azurerm_resource_group.jump_host_connector.name
  location                   = azurerm_resource_group.jump_host_connector.location
  service_plan_id            = azurerm_service_plan.jump_host_connector.id
  storage_account_name       = azurerm_storage_account.jump_host_connector.name
  storage_account_access_key = azurerm_storage_account.jump_host_connector.primary_access_key

  # System-assigned identity that the Run Command role is bound to below.
  identity {
    type = "SystemAssigned"
  }

  site_config {
    application_stack {
      docker {
        registry_url = "https://index.docker.io"
        image_name   = "p0security/p0-security-perimeter-azure-vm"
        image_tag    = "latest"
      }
    }
  }

  app_settings = {
    FUNCTIONS_WORKER_RUNTIME            = "node"
    CALLER_APP_ID                       = azuread_application_registration.session_terminator.client_id
    WEBSITES_ENABLE_APP_SERVICE_STORAGE = "false"
    AzureWebJobsFeatureFlags            = "EnableWorkerIndexing"
    # LOG_SECURITY_EVENTS: emit an audit event per terminated session.
    LOG_SECURITY_EVENTS = "true"
  }

  # Easy Auth: only callers with a token for the session-terminator app (P0) can invoke the connector.
  auth_settings_v2 {
    auth_enabled           = true
    require_authentication = true
    unauthenticated_action = "Return401"
    default_provider       = "azureactivedirectory"

    active_directory_v2 {
      client_id            = azuread_application_registration.session_terminator.client_id
      tenant_auth_endpoint = "https://login.microsoftonline.com/${local.directory_id}/v2.0"
      # Only tokens issued to and for this app are accepted; nothing else in the tenant can invoke it.
      allowed_audiences    = [azuread_application_registration.session_terminator.client_id]
      allowed_applications = [azuread_application_registration.session_terminator.client_id]
    }

    login {}
  }
}

# --- Federated credential for the session-terminator app ---
# Must exist before p0_azure_jump_host: that install does a real token exchange
# against this app and fails without the credential trusting P0's service account.
# subject is P0's root-install service account (reused here); issuer/audiences come
# from p0_azure.credential_info.
resource "azuread_application_federated_identity_credential" "session_terminator" {
  application_id = azuread_application_registration.session_terminator.id
  display_name   = p0_azure.example.credential_info.name
  description    = p0_azure.example.credential_info.description
  issuer         = p0_azure.example.credential_info.issuer
  audiences      = p0_azure.example.credential_info.audiences
  subject        = p0_azure.example.service_account_id
}

# --- Register jump host management with P0 ---
# Points P0 at the session-terminator app and the Function App; depends on the
# federated credential above so the install's token exchange succeeds.
resource "p0_azure_jump_host" "example" {
  depends_on = [
    p0_azure.example,
    p0_azure_app.example,
    azuread_application_federated_identity_credential.session_terminator,
  ]

  client_id                = azuread_application_registration.session_terminator.client_id
  function_app_resource_id = azurerm_linux_function_app.jump_host_connector.id
}

# --- Run Command role for the Function App identity ---
# The connector invokes Azure Run Command on jump host VMs, so its identity needs read + runCommand.
resource "azurerm_role_definition" "p0_run_command" {
  name  = "P0 Jump Host Run Command"
  scope = "/subscriptions/${local.subscription_id}"
  permissions {
    actions = [
      "Microsoft.Compute/virtualMachines/read",
      "Microsoft.Compute/virtualMachines/runCommand/action",
      "Microsoft.Compute/virtualMachines/runCommands/write",
    ]
  }
  assignable_scopes = ["/subscriptions/${local.subscription_id}"]
}

resource "azurerm_role_assignment" "function_app_run_command" {
  scope              = "/subscriptions/${local.subscription_id}"
  role_definition_id = azurerm_role_definition.p0_run_command.role_definition_resource_id
  principal_id       = azurerm_linux_function_app.jump_host_connector.identity[0].principal_id
  # The managed identity may not have propagated to Azure AD yet at apply time.
  skip_service_principal_aad_check = true
}
