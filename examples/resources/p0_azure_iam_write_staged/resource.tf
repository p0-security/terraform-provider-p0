resource "p0_azure" "example" {
  directory_id = "12345678-1234-1234-1234-123456789012"
  client_id    = "12345678-1234-1234-1234-123456789012"
}

resource "p0_azure_iam_write_staged" "example" {
  depends_on          = [p0_azure.example]
  management_group_id = "my-management-group"
}
