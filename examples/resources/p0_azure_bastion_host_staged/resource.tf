locals {
  directory_id    = "12345678-1234-1234-1234-123456789012"
  subscription_id = "12345678-1234-1234-1234-123456789012"
}

variable "target_ssh_public_key" {
  type = string
}

provider "azurerm" {
  features {}
  subscription_id = local.subscription_id
}

resource "p0_azure" "example" {
  directory_id = local.directory_id
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

# After apply, use custom_role to define your Bastion Host Management role, deploy
# Bastion, then register via p0_azure_bastion_host — see examples/resources/p0_azure_bastion_host/.

# --- Target VM prerequisites ---
# VMs reached through the Bastion over Azure IAM need the AADSSHLoginForLinux
# extension (which requires a managed identity) and a running SSH server (Ubuntu
# ships sshd enabled). Unlike a jump host, a Bastion target needs no public IP.
resource "azurerm_resource_group" "target" {
  name     = "p0-bastion-target"
  location = "eastus"
}

resource "azurerm_virtual_network" "target" {
  name                = "p0-bastion-target-vnet"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.target.location
  resource_group_name = azurerm_resource_group.target.name
}

resource "azurerm_subnet" "target" {
  name                 = "p0-bastion-target-subnet"
  resource_group_name  = azurerm_resource_group.target.name
  virtual_network_name = azurerm_virtual_network.target.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_network_interface" "target" {
  name                = "p0-bastion-target-nic"
  location            = azurerm_resource_group.target.location
  resource_group_name = azurerm_resource_group.target.name

  # No public IP: the Bastion reaches this VM over the private network.
  ip_configuration {
    name                          = "primary"
    subnet_id                     = azurerm_subnet.target.id
    private_ip_address_allocation = "Dynamic"
  }
}

resource "azurerm_linux_virtual_machine" "target" {
  name                  = "p0-bastion-target"
  location              = azurerm_resource_group.target.location
  resource_group_name   = azurerm_resource_group.target.name
  size                  = "Standard_B1s"
  admin_username        = "azureuser"
  network_interface_ids = [azurerm_network_interface.target.id]

  # AADSSHLoginForLinux requires the VM to have a managed identity.
  identity {
    type = "SystemAssigned"
  }

  admin_ssh_key {
    username   = "azureuser"
    public_key = var.target_ssh_public_key
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

# AADSSHLoginForLinux lets P0 authenticate SSH to this VM via Azure IAM.
resource "azurerm_virtual_machine_extension" "target_aad_login" {
  name                       = "AADSSHLoginForLinux"
  virtual_machine_id         = azurerm_linux_virtual_machine.target.id
  publisher                  = "Microsoft.Azure.ActiveDirectory"
  type                       = "AADSSHLoginForLinux"
  type_handler_version       = "1.0"
  auto_upgrade_minor_version = true
}
