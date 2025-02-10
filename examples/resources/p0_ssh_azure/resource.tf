locals {
  management_group_id = "my-management-group"
  subscription_id     = "12345678-1234-1234-1234-123456789012"
  bastion_id          = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/sample-resource-group/providers/Microsoft.Network/bastionHosts/sample-bastion"

}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

resource "azurerm_role_definition" "vm_admin_access" {
  name        = "Virtual Machine Administrator Access"
  scope       = "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  description = "Grants a user read access to virtual machines and Sudo SSH access"

  permissions {
    actions = [
      "Microsoft.Compute/virtualMachines/read",
      "Microsoft.Network/networkInterfaces/read",
      "Microsoft.Network/bastionHosts/read"
    ]
    data_actions = [
      "Microsoft.Compute/virtualMachines/loginAsAdmin/action",
      "Microsoft.Compute/virtualMachines/login/action"
    ]
  }

  assignable_scopes = [
    "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  ]
}

resource "azurerm_role_definition" "vm_standard_access" {
  name        = "Virtual Machine Standard Access"
  scope       = "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  description = "Grants a user read access to virtual machines and SSH access"

  permissions {
    actions = [
      "Microsoft.Compute/virtualMachines/read",
      "Microsoft.Network/networkInterfaces/read",
      "Microsoft.Network/bastionHosts/read"
    ]
    data_actions = [
      "Microsoft.Compute/virtualMachines/login/action"
    ]
  }

  assignable_scopes = [
    "/providers/Microsoft.Management/managementGroups/${local.management_group_id}"
  ]
}

resource "p0_ssh_azure" "example" {
  depends_on              = [azurerm_role_definition.vm_admin_access, azurerm_role_definition.vm_standard_access]
  admin_access_role_id    = azurerm_role_definition.vm_admin_access.role_definition_resource_id
  standard_access_role_id = azurerm_role_definition.vm_standard_access.role_definition_resource_id
  is_sudo_enabled         = true
  group_key               = "resource-group"
  bastion_id              = local.bastion_id
  management_group_id     = local.management_group_id
}
