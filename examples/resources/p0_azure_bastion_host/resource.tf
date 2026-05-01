locals {
  subscription_id = "12345678-1234-1234-1234-123456789012"
  # From your Bastion deployment (for example module.azure_p0_bastion.bastion_resource_id)
  bastion_id = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/example/providers/Microsoft.Network/bastionHosts/example"
  # From the Azure custom role you create from p0_azure_bastion_host_staged outputs, for example:
  #   name             = p0_azure_bastion_host_staged.example.custom_role.name
  #   description      = p0_azure_bastion_host_staged.example.custom_role.description
  #   actions          = p0_azure_bastion_host_staged.example.custom_role.actions
  #   assignable_scope = p0_azure_bastion_host_staged.example.custom_role.assignable_scope
  # then pass azurerm_role_definition (or module) ID here:
  bastion_role_definition_id = "/subscriptions/12345678-1234-1234-1234-123456789012/providers/Microsoft.Authorization/roleDefinitions/00000000-0000-0000-0000-000000000000"
}

resource "p0_azure" "example" {
  directory_id = "12345678-1234-1234-1234-123456789012"
}

resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = "12345678-1234-1234-1234-123456789012"
}

resource "p0_azure_iam_write" "example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.subscription_id
}

resource "p0_azure_bastion_host_staged" "example" {
  depends_on = [
    p0_azure.example,
    p0_azure_app.example,
    p0_azure_iam_write.example,
  ]
  subscription_id = local.subscription_id
}

resource "p0_azure_bastion_host" "example" {
  depends_on = [p0_azure_bastion_host_staged.example]

  subscription_id    = p0_azure_bastion_host_staged.example.subscription_id
  bastion_id         = local.bastion_id
  role_definition_id = local.bastion_role_definition_id
}
