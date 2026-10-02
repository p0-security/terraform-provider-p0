# Installs the Databricks connector: one Lambda per AWS account, outside any
# VPC, that P0 invokes to manage your Databricks accounts. It holds no secret:
# it exchanges an AWS identity token for each account's service principal.
# Apply it as an AWS admin of the connector's account.
# Full chain: p0_aws_iam_write -> p0_databricks_connector_staged -> outbound
# identity federation, the connector's role, Lambda and image, and P0's
# permission to invoke it -> p0_databricks_connector. Then add accounts (see p0_databricks_account).

terraform {
  # 1.4 added terraform_data.
  required_version = ">= 1.4"

  required_providers {
    # 6.26.0 added outbound identity federation.
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.26.0"
    }
    p0 = {
      source = "p0-security/p0"
    }
  }
}

locals {
  account_id = "123456789012"
  region     = "us-east-1"

  # The connector image's version: the sha- tag that P0 publishes each release
  # under. Bump it to roll out a new release, which copies the image again and
  # redeploys the Lambda. A digest pin (tag@sha256:...) follows once the first
  # image is published, as terraform-aws-p0-connector pins its images, because
  # a tag can be pushed again.
  connector_image_tag = "sha-b21deb4"
  connector_image     = "p0security/p0-connector-databricks:${local.connector_image_tag}"

  tags = {
    ManagedBy  = "Terraform"
    ManagedFor = "P0"
  }
}

provider "aws" {
  region              = local.region
  allowed_account_ids = [local.account_id]
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

# Lets the connector mint AWS identity tokens, which Databricks accepts in place
# of a secret. This is a setting of the whole AWS account: destroying this
# resource turns outbound identity federation off for everything in it. If it
# is already on, import it, with the AWS account ID as its ID, rather than
# create it:
#   terraform import aws_iam_outbound_web_identity_federation.this <account ID>
# To remove the connector but leave federation on, replace this block with a
# removed block (Terraform 1.7+) before you destroy the rest. Terraform then
# forgets the setting without turning it off:
#   removed {
#     from = aws_iam_outbound_web_identity_federation.this
#     lifecycle {
#       destroy = false
#     }
#   }
resource "aws_iam_outbound_web_identity_federation" "this" {}

# Each Databricks account's federation policy trusts this role by its ARN, so
# it must have this name.
resource "aws_iam_role" "connector" {
  name = "p0-connector-databricks"
  tags = local.tags
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "connector_logs" {
  role       = aws_iam_role.connector.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Only tokens for Databricks. Every Databricks federation policy that trusts
# this role expects this audience, "databricks".
resource "aws_iam_role_policy" "connector_identity_token" {
  name = "p0-connector-databricks-identity-token"
  role = aws_iam_role.connector.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sts:GetWebIdentityToken"
      Resource = "*"
      Condition = {
        "ForAllValues:StringEquals" = {
          "sts:IdentityTokenAudience" = [p0_databricks_connector_staged.example.federation_audience]
        }
      }
    }]
  })
}

# Lambda only runs images from Amazon ECR, so this copies P0's image into a
# repository in this account. It needs docker and the AWS CLI, signed in to this
# account, wherever Terraform runs.
resource "aws_ecr_repository" "connector" {
  name         = "p0-connector-databricks"
  force_delete = true
  tags         = local.tags

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Copies the image again whenever local.connector_image_tag changes.
resource "terraform_data" "connector_image" {
  triggers_replace = [aws_ecr_repository.connector.repository_url, local.connector_image]

  provisioner "local-exec" {
    command = <<-EOT
      aws ecr get-login-password --region ${aws_ecr_repository.connector.region} | docker login --username AWS --password-stdin ${split("/", aws_ecr_repository.connector.repository_url)[0]}
      docker pull --platform linux/amd64 ${local.connector_image}
      docker tag ${local.connector_image} ${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}
      docker push ${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}
    EOT
  }
}

data "aws_ecr_image" "connector" {
  repository_name = aws_ecr_repository.connector.name
  image_tag       = local.connector_image_tag

  depends_on = [terraform_data.connector_image]
}

resource "aws_lambda_function" "connector" {
  # P0 invokes the connector by this name.
  function_name = "p0-connector-databricks"
  role          = aws_iam_role.connector.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.connector.repository_url}@${data.aws_ecr_image.connector.image_digest}"
  architectures = ["x86_64"]
  timeout       = 30
  tags          = local.tags

  environment {
    variables = {
      # The connector refuses any user whose whole email domain doesn't match.
      DOMAIN_PATTERN = p0_databricks_connector_staged.example.domain_pattern
    }
  }
}

# Lets P0 invoke the connector, through the role P0's AWS integration assumes in
# this account.
resource "aws_iam_role_policy" "p0_invoke_connector" {
  name = "p0-connector-databricks-invoke"
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
    aws_iam_role_policy.p0_invoke_connector,
  ]
}
