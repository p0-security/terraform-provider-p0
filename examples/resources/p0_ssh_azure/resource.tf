locals {
  subscription_id = "12345678-1234-1234-1234-123456789012"
}

# The VM-access roles P0 assigns, and the Azure Bastion host or jump host P0
# connects through, are configured on the p0_azure_bastion_host component for
# the same subscription — not on this resource. Install p0_azure_bastion_host
# (and its prerequisites) before requesting access.
resource "p0_ssh_azure" "example" {
  subscription_id = local.subscription_id
  is_sudo_enabled = true
  group_key       = "resource-group"
}
