resource "p0_ssh_azure" "example" {
  management_group_id     = "sample-management-group"
  admin_access_role_id    = "/subscriptions/12345678-1234-1234-1234-123456789012/providers/Microsoft.Authorization/roleDefinitions/12345678-1234-1234-1234-123456789012"
  standard_access_role_id = "/subscriptions/12345678-1234-1234-1234-123456789012/providers/Microsoft.Authorization/roleDefinitions/12345678-1234-1234-1234-123456789012"
  bastion_id              = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/sample-resource-group/providers/Microsoft.Network/bastionHosts/sample-bastion"
}
