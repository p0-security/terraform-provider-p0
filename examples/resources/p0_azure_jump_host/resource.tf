resource "p0_azure" "example" {
  directory_id = "12345678-1234-1234-1234-123456789012"
}

resource "p0_azure_app" "example" {
  depends_on = [p0_azure.example]
  client_id  = "12345678-1234-1234-1234-123456789012"
}

# Registers jump host management: P0 authenticates as the app registration
# (via workload identity federation) and dispatches privileged commands to your
# jump hosts through the Azure Function App identified below.
resource "p0_azure_jump_host" "example" {
  depends_on = [
    p0_azure.example,
    p0_azure_app.example,
  ]

  client_id                = "12345678-1234-1234-1234-123456789012"
  function_app_resource_id = "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/p0-jump-hosts/providers/Microsoft.Web/sites/p0-kill-session"
}
