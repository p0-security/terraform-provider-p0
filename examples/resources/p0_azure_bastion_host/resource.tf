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

resource "p0_azure_bastion_host" "jump_host_example" {
  depends_on = [p0_azure_iam_write.jump_host_example]

  subscription_id = local.jump_host_subscription_id
  jump_host = {
    virtual_machine_id      = "/subscriptions/87654321-1234-1234-1234-123456789012/resourceGroups/example/providers/Microsoft.Compute/virtualMachines/example"
    standard_access_role_id = local.vm_user_login_role_id
    admin_access_role_id    = local.vm_admin_login_role_id
  }
}
