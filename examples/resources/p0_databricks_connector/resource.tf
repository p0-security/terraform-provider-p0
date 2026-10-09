# Installs the Databricks connector: one Lambda per AWS account, outside any
# VPC, that P0 invokes to manage your Databricks accounts. It holds no secret:
# it exchanges an AWS identity token for each account's service principal.
# Apply it as an AWS admin of the connector's account.
#
# Before you apply this, install P0's AWS IAM management for the connector's
# AWS account (see p0_aws_iam_write). P0 invokes the connector as that
# installation's role. Once the connector is installed, add Databricks
# accounts (see p0_databricks_account).

terraform {
  # 1.4 added terraform_data.
  required_version = ">= 1.4"

  required_providers {
    # 6.26.0 added outbound identity federation. This asks for 6.36.0, which
    # added its data source, to match the p0_databricks_account example.
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.36.0"
    }
    p0 = {
      source = "p0-security/p0"
    }
  }
}

locals {
  account_id = "123456789012"
  region     = "us-east-1"

  # The connector refuses any user whose whole email domain doesn't match this
  # regular expression. Changing it later updates only the Lambda: the P0
  # connector resources below ignore the change, so the connector isn't
  # replaced, and P0 keeps the pattern the connector was installed with.
  domain_pattern = "example\\.com"

  # The connector's name. P0 invokes it by this name, in the account and region
  # above, so keep it as it is. The connector's Lambda function, role, log group
  # and ECR repository all take it.
  connector_name = "p0-connector-databricks"

  # How many days CloudWatch keeps the connector's logs.
  log_retention_days = 30

  # The role that P0's AWS IAM management assumes in this account, which
  # invokes the connector.
  p0_role_name = "P0RoleIamManager"

  # The connector image's version: the sha- tag P0 publishes each release
  # under. To update, use the tag in the latest version of this example in the
  # Terraform registry. Changing it copies the image again and redeploys the
  # Lambda.
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

# Staging records the connector in P0 and checks its domain pattern before the
# Lambda is deployed with it.
resource "p0_databricks_connector_staged" "example" {
  id             = local.account_id
  region         = local.region
  domain_pattern = local.domain_pattern

  # P0 can't change a connector's domain pattern, so a new one would replace
  # the connector. A later change to local.domain_pattern updates only the
  # Lambda's DOMAIN_PATTERN.
  lifecycle {
    ignore_changes = [domain_pattern]
  }
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

# Each Databricks account's federation policy trusts this role by its ARN. The
# p0_databricks_account example finds the role by the connector's name.
resource "aws_iam_role" "connector" {
  name = local.connector_name
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

# The log group Lambda writes the connector's logs to.
resource "aws_cloudwatch_log_group" "connector" {
  name              = "/aws/lambda/${local.connector_name}"
  retention_in_days = local.log_retention_days
  tags              = local.tags
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "connector_logs" {
  name = "${local.connector_name}-logs"
  role = aws_iam_role.connector.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.connector.arn}:*"
    }]
  })
}

# Only tokens for Databricks. Every Databricks federation policy that trusts
# this role expects this audience, "databricks".
resource "aws_iam_role_policy" "connector_identity_token" {
  name = "${local.connector_name}-identity-token"
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
  name = local.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that signs in to Databricks as P0's service principal.
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true
  tags                 = local.tags

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Copies the image again whenever local.connector_image_tag changes, tagged
# with that tag and this copy's id. ECR's tags can't be overwritten, so each
# copy needs a tag of its own: copying the image again, to roll back or after
# an interrupted apply, pushes a new one.
resource "terraform_data" "connector_image" {
  triggers_replace = [aws_ecr_repository.connector.repository_url, local.connector_image]

  provisioner "local-exec" {
    # Signs Docker out of ECR at the end, so its login doesn't outlast the copy.
    # A failed sign-out doesn't fail the copy once the image is pushed.
    command = <<-EOT
      set -e
      trap 'docker logout "$REGISTRY" || true' EXIT
      aws ecr get-login-password --region "$REGION" |
        docker login --username AWS --password-stdin "$REGISTRY"
      docker pull --platform linux/amd64 "$SOURCE"
      docker tag "$SOURCE" "$TARGET"
      docker push "$TARGET"
    EOT

    environment = {
      REGION   = local.region
      REGISTRY = split("/", aws_ecr_repository.connector.repository_url)[0]
      SOURCE   = local.connector_image
      TARGET   = "${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}-${self.id}"
    }
  }
}

data "aws_ecr_image" "connector" {
  repository_name = aws_ecr_repository.connector.name
  image_tag       = "${local.connector_image_tag}-${terraform_data.connector_image.id}"
}

resource "aws_lambda_function" "connector" {
  # P0 invokes the connector by its name in this account and region, so it must
  # be deployed exactly there. If your other AWS providers have credential
  # settings, such as profile or assume_role, add them to the provider above.
  function_name = local.connector_name
  role          = aws_iam_role.connector.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.connector.repository_url}@${data.aws_ecr_image.connector.image_digest}"
  architectures = ["x86_64"]
  # Lambda's default of 3 seconds is too short for the connector to exchange
  # its AWS identity token with Databricks and then call Databricks. P0's
  # installer sets the same timeout.
  timeout = 30
  tags    = local.tags

  environment {
    variables = {
      # The connector refuses any user whose whole email domain doesn't match.
      # Read from the local, not from the staged connector, which keeps the
      # pattern it was installed with.
      DOMAIN_PATTERN = local.domain_pattern
    }
  }

  # P0 checks the domain pattern when it stages the connector. The connector's
  # role can't create a log group, so its own must exist before it runs.
  depends_on = [
    p0_databricks_connector_staged.example,
    aws_cloudwatch_log_group.connector,
  ]
}

# Lets P0 invoke the connector, through the role P0's AWS IAM management
# assumes in this account.
resource "aws_iam_role_policy" "p0_invoke_connector" {
  name = "${local.connector_name}-invoke"
  role = local.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.connector.arn
    }]
  })
}

# Completes the install. Like the staged connector, it ignores later changes to
# the domain pattern.
resource "p0_databricks_connector" "example" {
  id             = p0_databricks_connector_staged.example.id
  region         = p0_databricks_connector_staged.example.region
  domain_pattern = p0_databricks_connector_staged.example.domain_pattern

  lifecycle {
    ignore_changes = [domain_pattern]
  }

  depends_on = [
    aws_iam_outbound_web_identity_federation.this,
    aws_iam_role_policy.connector_identity_token,
    aws_iam_role_policy.p0_invoke_connector,
  ]
}
