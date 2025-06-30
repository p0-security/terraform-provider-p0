
locals {
  management_group_id = "my-management-group"
  # The tenant or directory ID for the Azure Installation
  directory_id = "12345678-1234-1234-1234-123456789012"
  # The billing subscription ID
  subscription_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.directory_id
}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

resource "azurerm_role_definition" "p0_service_management" {
  name        = "P0 Service Management"
  scope       = "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  description = "Gives P0 Access to manage access to virtual machines"

  permissions {
    actions = [
      "Microsoft.Management/managementGroups/read",
      "Microsoft.Management/managementGroups/subscriptions/read",
      "Microsoft.Authorization/roleAssignments/write",
      "Microsoft.Authorization/roleAssignments/delete",
      "Microsoft.Authorization/roleAssignments/read",
    ]
  }

  assignable_scopes = [
    "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  ]
}

resource "azuread_application_registration" "example" {
  display_name = "my-terraform-app"
}

resource "p0_azure" "example" {
  depends_on   = [azuread_application_registration.example]
  directory_id = local.directory_id
}

resource "azuread_application_federated_identity_credential" "p0_integration" {
  depends_on     = [p0_azure.example, azuread_application_registration.example]
  application_id = azuread_application_registration.example.id
  display_name   = "P0Integration"
  description    = "P0 integration with Azure"
  issuer         = "https://accounts.google.com"
  subject        = p0_azure.example.service_account_id
  audiences      = ["api://AzureADTokenExchange"]
}

resource "azuread_service_principal" "example" {
  depends_on = [azuread_application_registration.example]
  client_id  = azuread_application_registration.example.client_id
}

