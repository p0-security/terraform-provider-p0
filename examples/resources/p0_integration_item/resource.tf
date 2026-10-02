# Completes the function-caller staged in the p0_integration_item_staged example,
# once the role P0 assumes exists. Creating this verifies that P0 can assume it.
resource "p0_integration_item" "function_caller" {
  integration = p0_integration_item_staged.function_caller.integration
  component   = p0_integration_item_staged.function_caller.component
  id          = p0_integration_item_staged.function_caller.id

  depends_on = [
    aws_iam_role_policy.p0_invoke,
    aws_iam_role_policy.p0_read_role,
  ]
}

# An item that needs nothing provisioned first does not need a staged resource.
# Here, an integration's IAM-management item is pointed at the function above.
resource "p0_integration_item" "example" {
  integration = "my-integration"
  component   = "iam-write"
  id          = "primary"

  config = jsonencode({
    service = {
      type   = "aws"
      lambda = "aws:${p0_integration_item.function_caller.id}"
    }
  })
}
