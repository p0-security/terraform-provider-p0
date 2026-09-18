# Registers a custom application connector you built with @p0security/connector-sdk
# and deployed yourself. P0 never provisions the connector; this resource only names
# it and verifies that P0 can invoke it.
#
# This example hosts the connector on AWS Lambda. For Google Cloud Run, see gcp.tf
# in this directory.
#
# Full chain: p0_aws_iam_write (P0's role in the account) -> the connector's Lambda
# function -> lambda:InvokeFunction granted to P0's role -> p0_app.

locals {
  account_id     = "123456789012"
  region         = "us-east-1"
  connector_name = "p0-custom-app-connector"
}

# P0 assumes this role to invoke the connector, so the AWS integration must be
# installed for the connector's account. Stage it to obtain the role's name and
# policies, create the role, then install (mirrors the p0_aws_iam_write example).
resource "p0_aws_iam_write_staged" "example" {
  id = local.account_id
}

resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example.role.name
  assume_role_policy = p0_aws_iam_write_staged.example.role.trust_policy
}

resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example.role.inline_policy
}

resource "p0_aws_iam_write" "example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [aws_iam_role_policy.p0_iam_manager]

  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}

# The connector's own execution role. It needs whatever access your application
# requires, plus CloudWatch Logs.
resource "aws_iam_role" "connector" {
  name = "${local.connector_name}-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "connector_logs" {
  role       = aws_iam_role.connector.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Build the connector bundle outside Terraform, then point this at the archive.
# `newCustomAppLambdaHandler` exports the handler from the bundle's entrypoint.
resource "aws_lambda_function" "connector" {
  function_name    = local.connector_name
  role             = aws_iam_role.connector.arn
  filename         = var.connector_package
  source_code_hash = filebase64sha256(var.connector_package)
  handler          = "index.handler"
  runtime          = "nodejs22.x"
  timeout          = 60
  memory_size      = 512

  depends_on = [aws_iam_role_policy_attachment.connector_logs]
}

variable "connector_package" {
  description = "Path to the connector's built deployment archive"
  type        = string
  default     = "connector.zip"
}

# The only permission P0 needs on the connector.
resource "aws_iam_role_policy" "invoke_connector" {
  name = "InvokeCustomConnector"
  role = aws_iam_role.p0_iam_manager.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "InvokeCustomConnector"
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.connector.arn
    }]
  })
}

# Completes the install; creating it verifies the connector is reachable.
resource "p0_app" "example" {
  id = "internal-admin-tool"

  hosting = {
    type             = "aws"
    account_id       = local.account_id
    connector_name   = aws_lambda_function.connector.function_name
    connector_region = local.region
  }

  depends_on = [
    p0_aws_iam_write.example,
    aws_iam_role_policy.invoke_connector,
  ]
}
