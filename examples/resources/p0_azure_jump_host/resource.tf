locals {
  # Distinct placeholders so the two app registrations don't blur together:
  # `directory_id` is your Azure tenant, `subscription_id` is where the jump
  # hosts and the Function App connector live, and `p0_app_client_id` is the P0
  # root app registration (created in examples/resources/p0_azure_app/), which is
  # NOT the session-terminator app registration created below.
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

# The P0 root app registration P0 authenticates as for the tenant. This is a
# different app than the session-terminator app registration below; create it
# per examples/resources/p0_azure_app/ and pass its client ID here.
resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = local.p0_app_client_id
}

# --- Session-terminator app registration ---
# P0 authenticates as this app (via workload identity federation) when it calls
# the Function App to dispatch privileged commands to your jump hosts. Its
# federated credential is created below, before the jump-host registration,
# because p0_azure_jump_host's install performs a real token exchange against
# this app and fails if the credential trusting P0's service account is absent.
resource "azuread_application_registration" "session_terminator" {
  display_name = "P0 Session Terminator"
  # v2 access tokens, matching the tenant_auth_endpoint used by Easy Auth below.
  requested_access_token_version = 2
}

resource "azuread_service_principal" "session_terminator" {
  client_id = azuread_application_registration.session_terminator.client_id
}

# Exposes the app via an api:// identifier URI so it can be the audience of the
# tokens the Function App's Easy Auth validates.
resource "azuread_application_identifier_uri" "session_terminator" {
  application_id = azuread_application_registration.session_terminator.id
  identifier_uri = "api://${azuread_application_registration.session_terminator.client_id}"
}

# --- Function App connector ---
# The Function App runs P0's session-terminator package, which uses Azure Run
# Command to terminate live SSH sessions on your jump host VMs. It needs a
# storage account, an Elastic Premium (EP1) Linux plan, a system-assigned
# identity (granted the Run Command role at the bottom of this file), and Easy
# Auth locked to the session-terminator app so only P0 can invoke it.
resource "azurerm_resource_group" "jump_host_connector" {
  name     = "p0-jump-host-connector"
  location = local.location
}

# Storage account name must be globally unique, 3-24 lowercase alphanumeric
# characters; change this before applying.
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
    # Runs P0's published session-terminator container image.
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
    # P0's session-terminator package emits an audit event for each terminated
    # session when this is enabled.
    LOG_SECURITY_EVENTS = "true"
  }

  # Easy Auth: reject any caller not presenting a token for the
  # session-terminator app, so only P0 can invoke the connector.
  auth_settings_v2 {
    auth_enabled           = true
    require_authentication = true
    unauthenticated_action = "Return401"
    default_provider       = "azureactivedirectory"

    active_directory_v2 {
      client_id            = azuread_application_registration.session_terminator.client_id
      tenant_auth_endpoint = "https://login.microsoftonline.com/${local.directory_id}/v2.0"
      # Lock Easy Auth to the session-terminator app itself: only tokens issued
      # to (and for) this app registration are accepted, so nothing else in the
      # tenant can invoke the connector.
      allowed_audiences    = [azuread_application_registration.session_terminator.client_id]
      allowed_applications = [azuread_application_registration.session_terminator.client_id]
    }

    login {}
  }
}

# --- Federated credential for the session-terminator app ---
# Must exist before p0_azure_jump_host: that install authenticates to this app
# via workload identity federation (a real token exchange) and fails unless the
# credential trusting P0's service account is already in place. The subject is
# P0's service account from the root install (the jump-host install reuses the
# same service account, so p0_azure.example.service_account_id is correct);
# issuer and audiences come from p0_azure.credential_info.
resource "azuread_application_federated_identity_credential" "session_terminator" {
  application_id = azuread_application_registration.session_terminator.id
  display_name   = p0_azure.example.credential_info.name
  description    = p0_azure.example.credential_info.description
  issuer         = p0_azure.example.credential_info.issuer
  audiences      = p0_azure.example.credential_info.audiences
  subject        = p0_azure.example.service_account_id
}

# --- Register jump host management with P0 ---
# Points P0 at the session-terminator app (client_id) and the Function App
# (function_app_resource_id). Depends on the federated credential so the
# install's token exchange against the session-terminator app succeeds.
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
# The connector terminates SSH sessions by invoking Azure Run Command on your
# jump host VMs, so its managed identity needs read + runCommand permissions.
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
