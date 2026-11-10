# Stage the function caller: computes the role P0 assumes to invoke the function,
# in the function's account.
resource "p0_aws_function_caller_staged" "example" {
  id = "arn:aws:lambda:us-west-2:123456789012:function:p0-notifier"
}

resource "aws_iam_role" "p0_function_caller" {
  name               = p0_aws_function_caller_staged.example.role.name
  assume_role_policy = p0_aws_function_caller_staged.example.role.trust_policy
}

# Allows the role to invoke the function, and nothing else.
resource "aws_iam_role_policy" "p0_function_caller" {
  name   = p0_aws_function_caller_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_function_caller.name
  policy = p0_aws_function_caller_staged.example.role.inline_policy
}

# Lets P0 read the role, so it can check it at install.
resource "aws_iam_role_policy" "p0_read_role" {
  name   = p0_aws_function_caller_staged.example.role.reader_policy_name
  role   = aws_iam_role.p0_function_caller.name
  policy = p0_aws_function_caller_staged.example.role.reader_policy
}

# The p0_aws_function_caller example completes the install.
