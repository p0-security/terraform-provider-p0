# Stages an AWS Lambda function-caller, so that its metadata can be used to create
# the role P0 assumes to invoke the function. Complete the install with a
# p0_integration_item for the same item (see that resource's example).
locals {
  account_id    = "123456789012"
  function_name = "my-function"
  function_arn  = "arn:aws:lambda:us-west-2:${local.account_id}:function:${local.function_name}"
}

resource "p0_integration_item_staged" "function_caller" {
  integration = "aws"
  component   = "function-caller"
  id          = local.function_arn
}

# One role serves every function-caller in an account; for a second function, add
# only another invoke policy.
resource "aws_iam_role" "p0_function_caller" {
  name               = p0_integration_item_staged.function_caller.metadata.roleName
  assume_role_policy = p0_integration_item_staged.function_caller.metadata.trustPolicy
}

# P0 verifies this policy by name, which must be "P0Invoke" followed by the
# function's name.
resource "aws_iam_role_policy" "p0_invoke" {
  name   = "P0Invoke${local.function_name}"
  role   = aws_iam_role.p0_function_caller.name
  policy = p0_integration_item_staged.function_caller.metadata.inlinePolicy
}

# Lets P0 read the role back while verifying the install.
resource "aws_iam_role_policy" "p0_read_role" {
  name = "P0CanReadRolesPolicy"
  role = aws_iam_role.p0_function_caller.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "P0CanListAccountAliases"
        Effect    = "Allow"
        Action    = ["iam:ListAccountAliases"]
        Resource  = "*"
        Condition = { StringEquals = { "aws:ResourceAccount" = local.account_id } }
      },
      {
        Sid       = "P0CanReadOwnPolicyForValidation"
        Effect    = "Allow"
        Action    = ["iam:GetRole", "iam:ListRoles", "iam:GetRolePolicy"]
        Resource  = [aws_iam_role.p0_function_caller.arn]
        Condition = { StringEquals = { "aws:ResourceAccount" = local.account_id } }
      },
    ]
  })
}
