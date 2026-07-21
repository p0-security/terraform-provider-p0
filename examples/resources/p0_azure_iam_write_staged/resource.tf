locals {
  # The tenant or directory ID for the Azure installation.
  directory_id = "12345678-1234-1234-1234-123456789012"
  # The subscription P0 will manage IAM grants in.
  subscription_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.directory_id
}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

# 1. Register the P0 Azure integration for the tenant.
resource "p0_azure" "example" {
  directory_id = local.directory_id
}

# 2. Complete the P0 app registration. Creating the Azure AD application, its
#    federated identity credential, and the required Microsoft Graph
#    (User.Read.All) grant is shown end-to-end in
#    examples/resources/p0_azure_iam_write/resource.tf (and
#    examples/resources/p0_azure_app_staged/); set client_id here to that
#    application's client ID once it exists.
resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = "12345678-1234-1234-1234-123456789012"
}

# Resolve the object ID of P0's service principal so the custom role can be
# assigned to it below.
data "azuread_service_principal" "p0" {
  client_id = p0_azure_app.example.client_id
}

# 3. Stage the IAM-write install to obtain the custom role P0 needs to manage
#    role assignments in the subscription. The role spec is computed into
#    custom_role (name, description, actions, assignable_scope, condition).
resource "p0_azure_iam_write_staged" "example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.subscription_id
}

# 4. Materialize that custom role from the staged spec and assign it to P0's
#    service principal, so P0 can assign and remove roles in the subscription.
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

  # To constrain P0 to specific roles or principals, apply the staged
  # custom_role.condition here (with condition_version = "2.0") when it is set.
}

# 5. Complete the install once the role assignment exists: apply
#    p0_azure_iam_write with the same subscription_id. See
#    examples/resources/p0_azure_iam_write/resource.tf for the full
#    end-to-end chain.
