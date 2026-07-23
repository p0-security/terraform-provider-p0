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

resource "p0_azure" "example" {
  directory_id = local.directory_id
}

# Complete the app registration: create the Azure AD app, its federated
# credential, and the User.Read.All grant (shown end-to-end in
# examples/resources/p0_azure_iam_write/resource.tf), then set client_id here.
resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = "12345678-1234-1234-1234-123456789012"
}

# P0's service principal object ID, for the role assignment below.
data "azuread_service_principal" "p0" {
  client_id = p0_azure_app.example.client_id
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
  principal_id       = data.azuread_service_principal.p0.object_id

  # ABAC condition blocking P0 from assigning/revoking roles for its own service
  # principal (privilege escalation); P0 always returns it in the staged custom_role.
  condition         = p0_azure_iam_write_staged.example.custom_role.condition
  condition_version = "2.0"
}

# Complete the install by applying p0_azure_iam_write with the same
# subscription_id; see examples/resources/p0_azure_iam_write/resource.tf.
