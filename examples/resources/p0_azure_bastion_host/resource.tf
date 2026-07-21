locals {
  subscription_id = "12345678-1234-1234-1234-123456789012"
  # A subscription uses either an Azure Bastion host or a jump host, never both,
  # so the jump host example below uses a second subscription.
  jump_host_subscription_id = "87654321-1234-1234-1234-123456789012"
  # From your Bastion deployment (for example module.azure_p0_bastion.bastion_resource_id)
  bastion_id = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/example/providers/Microsoft.Network/bastionHosts/example"

  # The VM-access roles P0 assigns to a connecting user. Point each at a
  # built-in role, an existing custom role, or a new one. The built-in
  # "Virtual Machine User Login" / "Virtual Machine Administrator Login" roles
  # are the recommended defaults; their IDs are stable across every tenant.
  vm_user_login_role_id  = "/providers/Microsoft.Authorization/roleDefinitions/fb879df8-f326-4884-b1cf-06f3ad86be52"
  vm_admin_login_role_id = "/providers/Microsoft.Authorization/roleDefinitions/1c0163c0-47e6-4577-8991-ea5c82e286e4"
}

# The SSH public key installed on the jump host's admin user.
variable "jump_host_ssh_public_key" {
  type = string
}

# Only the jump host option below provisions Azure infrastructure through the
# azurerm provider, so this provider targets the jump host's subscription. The
# Azure Bastion option references an already-deployed Bastion by ID and needs no
# azurerm resources here.
provider "azurerm" {
  features {}
  subscription_id = local.jump_host_subscription_id
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

# Option 1: a managed Azure Bastion host. Stage first to obtain the custom role
# spec, create the P0 Bastion Host Management role and the Bastion in Azure, then
# register the Bastion ID and the VM-access roles. P0 verifies the Bastion Host
# Management role by name, so its ID is not passed here.
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

  subscription_id = p0_azure_bastion_host_staged.example.subscription_id
  azure_bastion = {
    bastion_id              = local.bastion_id
    standard_access_role_id = local.vm_user_login_role_id
    admin_access_role_id    = local.vm_admin_login_role_id
  }
}

# Option 2: a customer-managed jump host VM. No staged resource or Bastion host
# is needed; the VM must have a public IP on its primary network interface,
# which P0 resolves and stores at install time. standard_access_role_id and
# admin_access_role_id are the roles granted to users connecting through the
# jump host (a built-in role, an existing custom role, or a new one).
resource "p0_azure_iam_write" "jump_host_example" {
  depends_on      = [p0_azure_app.example]
  subscription_id = local.jump_host_subscription_id
}

# --- Jump host VM prerequisites ---
# P0 reaches the jump host over its public IP and authenticates SSH sessions
# through Azure IAM, so the VM must have:
#   - a public IP address on its primary network interface,
#   - the Azure AD login for Linux extension (AADSSHLoginForLinux), which in
#     turn requires the VM to have a managed identity, and
#   - a running SSH server. Ubuntu marketplace images ship sshd enabled.
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

  # The public IP on the primary IP configuration is what P0 resolves at
  # install time to reach the jump host.
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

  # AADSSHLoginForLinux authenticates SSH through Azure AD and requires the VM
  # to have a managed identity.
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

  # Ubuntu ships and enables sshd by default, satisfying the SSH-server
  # requirement.
  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }
}

# Install the Azure AD login for Linux extension so P0 can authenticate SSH
# sessions to the jump host through Azure IAM.
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
