# The Lambda function that P0 invokes with each notification event.
resource "aws_iam_role" "notifier" {
  name = "p0-notifier"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_lambda_function" "notifier" {
  function_name = "p0-notifier"
  role          = aws_iam_role.notifier.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "notifier.zip"
}

# The role P0 assumes to invoke the function, with permission to invoke it.
resource "p0_aws_function_caller_staged" "notifier" {
  id = aws_lambda_function.notifier.arn
}

resource "aws_iam_role" "p0_function_caller" {
  name               = p0_aws_function_caller_staged.notifier.role.name
  assume_role_policy = p0_aws_function_caller_staged.notifier.role.trust_policy
}

resource "aws_iam_role_policy" "p0_function_caller" {
  name   = p0_aws_function_caller_staged.notifier.role.inline_policy_name
  role   = aws_iam_role.p0_function_caller.name
  policy = p0_aws_function_caller_staged.notifier.role.inline_policy
}

resource "p0_aws_function_caller" "notifier" {
  id         = p0_aws_function_caller_staged.notifier.id
  depends_on = [aws_iam_role_policy.p0_function_caller]
}

# The notifier. Referencing the function caller's id creates it after the
# function caller is installed.
resource "p0_aws_lambda_notifier" "example" {
  id              = "p0-notifier"
  function_caller = p0_aws_function_caller.notifier.id
}
