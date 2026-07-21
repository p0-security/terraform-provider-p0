locals {
  directory_id    = "12345678-1234-1234-1234-123456789012"
  subscription_id = "12345678-1234-1234-1234-123456789012"
  # A subscription uses either a Bastion host or a jump host, never both; the jump host example uses a second subscription.
  jump_host_subscription_id = "87654321-1234-1234-1234-123456789012"
  # From your Bastion deployment (for example module.azure_p0_bastion.bastion_resource_id)
  bastion_id = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/example/providers/Microsoft.Network/bastionHosts/example"

  # VM-access roles P0 assigns to a connecting user: a built-in role, an existing
  # custom role, or a new one. The built-in "Virtual Machine User/Administrator
  # Login" roles are the recommended defaults; their IDs are stable across tenants.
  vm_user_login_role_id  = "/providers/Microsoft.Authorization/roleDefinitions/fb879df8-f326-4884-b1cf-06f3ad86be52"
  vm_admin_login_role_id = "/providers/Microsoft.Authorization/roleDefinitions/1c0163c0-47e6-4577-8991-ea5c82e286e4"
}

variable "jump_host_ssh_public_key" {
  type = string
}

# Only the jump host option provisions azurerm infrastructure, so this provider targets
# the jump host's subscription; the Bastion option references an existing Bastion by ID.
provider "azurerm" {
  features {}
  subscription_id = local.jump_host_subscription_id
}

provider "azuread" {
  tenant_id = local.directory_id
}

resource "p0_azure" "example" {
  directory_id = local.directory_id
}

resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = "12345678-1234-1234-1234-123456789012"
}

# P0's service principal object ID, for the Bastion role assignment below.
data "azuread_service_principal" "p0" {
  client_id = p0_azure_app.example.client_id
}

resource "p0_azure_iam_write" "example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.subscription_id
}

# Option 1: managed Azure Bastion host. Stage for the custom role spec, create the
# P0 Bastion Host Management role and the Bastion, then register the Bastion ID and
# VM-access roles. P0 verifies the role by name, so its ID isn't passed here.
resource "p0_azure_bastion_host_staged" "example" {
  depends_on = [
    p0_azure.example,
    p0_azure_app.example,
    p0_azure_iam_write.example,
  ]
  subscription_id = local.subscription_id
}

# Create the Bastion Host Management role from the staged spec and assign it to P0's SP.
resource "azurerm_role_definition" "p0_bastion" {
  name              = p0_azure_bastion_host_staged.example.custom_role.name
  description       = p0_azure_bastion_host_staged.example.custom_role.description
  scope             = p0_azure_bastion_host_staged.example.custom_role.assignable_scope
  assignable_scopes = [p0_azure_bastion_host_staged.example.custom_role.assignable_scope]

  permissions {
    actions = p0_azure_bastion_host_staged.example.custom_role.actions
  }
}

resource "azurerm_role_assignment" "p0_bastion" {
  scope              = p0_azure_bastion_host_staged.example.custom_role.assignable_scope
  role_definition_id = azurerm_role_definition.p0_bastion.role_definition_resource_id
  principal_id       = data.azuread_service_principal.p0.object_id
}

resource "p0_azure_bastion_host" "example" {
  depends_on = [
    p0_azure_bastion_host_staged.example,
    azurerm_role_assignment.p0_bastion,
  ]

  subscription_id = p0_azure_bastion_host_staged.example.subscription_id
  azure_bastion = {
    bastion_id              = local.bastion_id
    standard_access_role_id = local.vm_user_login_role_id
    admin_access_role_id    = local.vm_admin_login_role_id
  }
}

# Option 2: customer-managed jump host VM (no staged resource or Bastion). The VM
# must have a public IP on its primary NIC, which P0 resolves and stores at install.
# standard/admin_access_role_id are the roles granted to users connecting through it.
resource "p0_azure_iam_write" "jump_host_example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.jump_host_subscription_id
}

# --- Jump host VM prerequisites ---
# P0 reaches the jump host over its public IP and authenticates SSH via Azure IAM,
# so the VM needs: a public IP on its primary NIC; the AADSSHLoginForLinux extension
# (requires a managed identity); and a running SSH server (Ubuntu ships sshd enabled).
resource "azurerm_resource_group" "jump_host" {
  name     = "p0-jump-host"
  location = "eastus"
}

resource "azurerm_virtual_network" "jump_host" {
  name                = "p0-jump-host-vnet"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.jump_host.location
  resource_group_name = azurerm_resource_group.jump_host.name
}

resource "azurerm_subnet" "jump_host" {
  name                 = "p0-jump-host-subnet"
  resource_group_name  = azurerm_resource_group.jump_host.name
  virtual_network_name = azurerm_virtual_network.jump_host.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_public_ip" "jump_host" {
  name                = "p0-jump-host-ip"
  location            = azurerm_resource_group.jump_host.location
  resource_group_name = azurerm_resource_group.jump_host.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_network_interface" "jump_host" {
  name                = "p0-jump-host-nic"
  location            = azurerm_resource_group.jump_host.location
  resource_group_name = azurerm_resource_group.jump_host.name

  # P0 resolves this primary-config public IP at install time to reach the jump host.
  ip_configuration {
    name                          = "primary"
    subnet_id                     = azurerm_subnet.jump_host.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.jump_host.id
  }
}

resource "azurerm_linux_virtual_machine" "jump_host" {
  name                  = "p0-jump-host"
  location              = azurerm_resource_group.jump_host.location
  resource_group_name   = azurerm_resource_group.jump_host.name
  size                  = "Standard_B1s"
  admin_username        = "azureuser"
  network_interface_ids = [azurerm_network_interface.jump_host.id]

  # AADSSHLoginForLinux requires the VM to have a managed identity.
  identity {
    type = "SystemAssigned"
  }

  admin_ssh_key {
    username   = "azureuser"
    public_key = var.jump_host_ssh_public_key
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  # Ubuntu ships sshd enabled, satisfying the SSH-server requirement.
  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }
}

# AADSSHLoginForLinux lets P0 authenticate SSH to the jump host via Azure IAM.
resource "azurerm_virtual_machine_extension" "jump_host_aad_login" {
  name                       = "AADSSHLoginForLinux"
  virtual_machine_id         = azurerm_linux_virtual_machine.jump_host.id
  publisher                  = "Microsoft.Azure.ActiveDirectory"
  type                       = "AADSSHLoginForLinux"
  type_handler_version       = "1.0"
  auto_upgrade_minor_version = true
}

resource "p0_azure_bastion_host" "jump_host_example" {
  depends_on = [
    p0_azure_iam_write.jump_host_example,
    azurerm_virtual_machine_extension.jump_host_aad_login,
  ]

  subscription_id = local.jump_host_subscription_id
  jump_host = {
    virtual_machine_id      = azurerm_linux_virtual_machine.jump_host.id
    standard_access_role_id = local.vm_user_login_role_id
    admin_access_role_id    = local.vm_admin_login_role_id
  }
}
