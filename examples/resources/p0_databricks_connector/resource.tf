# Installs the Databricks connector: one Lambda per AWS account, outside any
# VPC, that P0 invokes to manage your Databricks accounts. It holds no secret:
# it exchanges an AWS identity token for each account's service principal.
# Full chain: p0_aws_iam_write -> p0_databricks_connector_staged -> the
# connector's Lambda and role, and outbound identity federation ->
# p0_databricks_connector. Then add accounts (see p0_databricks_account).

locals {
  account_id = "123456789012"
  region     = "us-east-1"
  # Terraform copies the connector image again only when this tag changes. Pin
  # a sha- tag to choose the version you deploy.
  connector_image_tag = "latest"
}

provider "aws" {
  region = local.region
}

# P0 invokes the connector as its AWS integration's role, so install that first
# (mirrors the p0_aws_iam_write example).
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

# Staging records the connector in P0 and checks its domain pattern before the
# Lambda is deployed with it.
resource "p0_databricks_connector_staged" "example" {
  id             = p0_aws_iam_write.example.id
  region         = local.region
  domain_pattern = "example\\.com"
}

# Lambda runs container images from ECR only, so copy P0's image into this
# account. This needs docker and the AWS CLI where you run Terraform.
resource "aws_ecr_repository" "connector" {
  name         = "p0-connector-databricks"
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "terraform_data" "connector_image" {
  triggers_replace = [aws_ecr_repository.connector.repository_url, local.connector_image_tag]

  provisioner "local-exec" {
    command = <<-EOT
      aws ecr get-login-password --region ${local.region} | docker login --username AWS --password-stdin ${split("/", aws_ecr_repository.connector.repository_url)[0]}
      docker pull --platform linux/amd64 p0security/p0-connector-databricks:${local.connector_image_tag}
      docker tag p0security/p0-connector-databricks:${local.connector_image_tag} ${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}
      docker push ${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}
    EOT
  }
}

data "aws_ecr_image" "connector" {
  repository_name = aws_ecr_repository.connector.name
  image_tag       = local.connector_image_tag
  depends_on      = [terraform_data.connector_image]
}

# The connector's execution role. Each Databricks account's federation policy
# trusts this role's ARN, which is the subject of the tokens the connector mints.
resource "aws_iam_role" "connector" {
  name = "p0-connector-databricks"

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

# The connector mints identity tokens for the "databricks" audience only.
resource "aws_iam_role_policy" "connector_identity_token" {
  name = "MintDatabricksIdentityToken"
  role = aws_iam_role.connector.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sts:GetWebIdentityToken"
      Resource = "*"
      Condition = {
        "ForAnyValue:StringEquals" = {
          "sts:IdentityTokenAudience" = ["databricks"]
        }
      }
    }]
  })
}

# Lets the account's roles mint identity tokens for services outside AWS. This
# is account-wide, and destroying this resource turns it off for the whole
# account. If it is already on, import this resource, or read the issuer with
# the data source of the same name instead.
resource "aws_iam_outbound_web_identity_federation" "this" {}

resource "aws_lambda_function" "connector" {
  # P0 invokes the connector by this name.
  function_name = "p0-connector-databricks"
  role          = aws_iam_role.connector.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.connector.repository_url}@${data.aws_ecr_image.connector.image_digest}"
  architectures = ["x86_64"]
  timeout       = 30

  environment {
    variables = {
      # The connector refuses every user whose email domain doesn't match.
      DOMAIN_ALLOW_PATTERN = p0_databricks_connector_staged.example.domain_pattern
    }
  }

  depends_on = [aws_iam_role_policy_attachment.connector_logs]
}

# The only permission P0 needs on the connector.
resource "aws_iam_role_policy" "invoke_connector" {
  name = "InvokeP0DatabricksConnector"
  role = aws_iam_role.p0_iam_manager.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.connector.arn
    }]
  })
}

# Completes the install.
resource "p0_databricks_connector" "example" {
  id             = p0_databricks_connector_staged.example.id
  region         = p0_databricks_connector_staged.example.region
  domain_pattern = p0_databricks_connector_staged.example.domain_pattern

  depends_on = [
    aws_iam_outbound_web_identity_federation.this,
    aws_iam_role_policy.connector_identity_token,
    aws_iam_role_policy.invoke_connector,
  ]
}
