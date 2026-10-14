# Installs P0's Databricks integration in one configuration: the connector in
# one AWS account, P0's service principal in one Databricks account, and that
# account's workspaces and catalogs. Each step refers to the one before it, so
# no ID is copied by hand.
#
# Before you apply this, install P0's AWS IAM management for the connector's
# AWS account (see p0_aws_iam_write). Apply it signed in to that AWS account as
# an admin, with docker and the AWS CLI working in the same shell, and signed in
# to Databricks as an account admin who also owns each catalog, holds MANAGE on
# it, or is a metastore admin.

terraform {
  # 1.4 added terraform_data.
  required_version = ">= 1.4"

  required_providers {
    # 6.26.0 added outbound identity federation.
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.26.0"
    }
    # 1.113.0 made databricks_grant honor provider_config with an account-level
    # provider, and 1.115.0 fixed the account-level reads that broke after it.
    databricks = {
      source  = "databricks/databricks"
      version = ">= 1.115.0"
    }
    p0 = {
      source = "p0-security/p0"
    }
  }
}

locals {
  # The AWS account and region the connector runs in.
  aws_account_id = "123456789012"
  aws_region     = "us-east-1"

  # The connector refuses any user whose whole email domain doesn't match this
  # regular expression. P0 keeps the pattern the connector was installed with,
  # so a later change reaches only the Lambda.
  domain_pattern = "example\\.com"

  # The role that P0's AWS IAM management assumes in this account, which
  # invokes the connector.
  p0_role_name = "P0RoleIamManager"

  # The connector image's version: the sha- tag P0 publishes each release
  # under. To update, use the tag in the latest p0_databricks_connector example
  # in the Terraform registry.
  connector_image_tag = "sha-e7a7816"
  connector_image     = "p0security/p0-connector-databricks:${local.connector_image_tag}"

  # P0 invokes the connector by this name, so keep it as it is.
  connector_name = "p0-connector-databricks"

  databricks_account_id = "01234567-89ab-cdef-0123-456789abcdef"
  accounts_url          = "https://accounts.cloud.databricks.com"

  # Whether users can request account admin. P0's service principal holds
  # account admin only while this is true.
  enable_account_admin = false

  # The workspaces to add, keyed by workspace ID. Each needs a Unity Catalog
  # metastore assigned.
  #   url:          https:// and the workspace's lowercase host, nothing after.
  #   name:         the workspace's name in P0, which requests show and
  #                 approval rules can match.
  #   enable_admin: whether users can request workspace admin. P0's service
  #                 principal is a workspace admin only while this is true.
  #   catalogs:     lowercase catalog names to add through this workspace.
  workspaces = {
    "1234567890123456" = {
      url          = "https://dbc-1234abcd-5678.cloud.databricks.com"
      name         = "analytics-prod"
      enable_admin = false
      catalogs     = ["main", "analytics"]
    }
  }

  # Keyed the way P0 keys catalogs: <catalog_name>@<workspace_id>.
  catalogs = merge([
    for workspace_id, workspace in local.workspaces : {
      for catalog in workspace.catalogs : "${catalog}@${workspace_id}" => {
        workspace_id = workspace_id
        catalog_name = catalog
      }
    }
  ]...)

  tags = {
    ManagedBy  = "Terraform"
    ManagedFor = "P0"
  }
}

provider "aws" {
  region              = local.aws_region
  allowed_account_ids = [local.aws_account_id]
}

# Service principals, workspace assignments and catalog grants are all made
# through the account-level provider.
provider "databricks" {
  host       = local.accounts_url
  account_id = local.databricks_account_id
}

# ---------------------------------------------------------------------------
# Connector (see the p0_databricks_connector example for each step's detail)
# ---------------------------------------------------------------------------

resource "p0_databricks_connector_staged" "this" {
  id             = local.aws_account_id
  region         = local.aws_region
  domain_pattern = local.domain_pattern

  # P0 can't change a connector's domain pattern, so a new one would replace
  # the connector.
  lifecycle {
    ignore_changes = [domain_pattern]
  }
}

# A setting of the whole AWS account: destroying it turns outbound identity
# federation off for everything in the account. If it is already on, import it
# with the AWS account ID as its ID.
resource "aws_iam_outbound_web_identity_federation" "this" {}

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

resource "aws_cloudwatch_log_group" "connector" {
  name              = "/aws/lambda/${local.connector_name}"
  retention_in_days = 30
  tags              = local.tags
}

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

# Only tokens for Databricks.
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
          "sts:IdentityTokenAudience" = [p0_databricks_connector_staged.this.federation_audience]
        }
      }
    }]
  })
}

# Lambda only runs images from Amazon ECR, so this copies P0's image into a
# repository in this account.
resource "aws_ecr_repository" "connector" {
  name                 = local.connector_name
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true
  tags                 = local.tags

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "terraform_data" "connector_image" {
  triggers_replace = [aws_ecr_repository.connector.repository_url, local.connector_image]

  provisioner "local-exec" {
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
      REGION   = local.aws_region
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
  function_name = local.connector_name
  role          = aws_iam_role.connector.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.connector.repository_url}@${data.aws_ecr_image.connector.image_digest}"
  architectures = ["x86_64"]
  timeout       = 30
  tags          = local.tags

  environment {
    variables = {
      # Read from the local, not from the staged connector, which keeps the
      # pattern it was installed with.
      DOMAIN_PATTERN = local.domain_pattern
    }
  }

  depends_on = [
    p0_databricks_connector_staged.this,
    aws_cloudwatch_log_group.connector,
  ]
}

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

resource "p0_databricks_connector" "this" {
  id             = p0_databricks_connector_staged.this.id
  region         = p0_databricks_connector_staged.this.region
  domain_pattern = p0_databricks_connector_staged.this.domain_pattern

  lifecycle {
    ignore_changes = [domain_pattern]
  }

  depends_on = [
    aws_iam_outbound_web_identity_federation.this,
    aws_iam_role_policy.connector_logs,
    aws_iam_role_policy.connector_identity_token,
    aws_iam_role_policy.p0_invoke_connector,
  ]
}

# ---------------------------------------------------------------------------
# Account (see the p0_databricks_account example)
# ---------------------------------------------------------------------------

resource "p0_databricks_account_staged" "this" {
  id           = local.databricks_account_id
  connector    = p0_databricks_connector.this.id
  accounts_url = local.accounts_url
}

resource "databricks_service_principal" "p0" {
  display_name = "p0-connector"
}

# Lets the connector's role sign in as this principal with an AWS identity
# token, so no secret is ever issued.
resource "databricks_service_principal_federation_policy" "p0" {
  service_principal_id = databricks_service_principal.p0.id
  oidc_policy = {
    issuer    = aws_iam_outbound_web_identity_federation.this.issuer_identifier
    subject   = aws_iam_role.connector.arn
    audiences = [p0_databricks_account_staged.this.federation_audience]
  }
}

resource "databricks_service_principal_role" "p0_account_admin" {
  count = local.enable_account_admin ? 1 : 0

  service_principal_id = databricks_service_principal.p0.id
  role                 = "account_admin"
}

resource "p0_databricks_account" "this" {
  id                       = p0_databricks_account_staged.this.id
  connector                = p0_databricks_account_staged.this.connector
  accounts_url             = p0_databricks_account_staged.this.accounts_url
  application_id           = databricks_service_principal.p0.application_id
  is_account_admin_enabled = local.enable_account_admin

  depends_on = [
    databricks_service_principal_federation_policy.p0,
    databricks_service_principal_role.p0_account_admin,
  ]
}

# ---------------------------------------------------------------------------
# Workspaces (see the p0_databricks_workspace example)
# ---------------------------------------------------------------------------

# USER is enough for Unity Catalog grants. Only workspace-admin requests need
# ADMIN.
resource "databricks_mws_permission_assignment" "p0" {
  for_each = local.workspaces

  workspace_id = each.key
  principal_id = databricks_service_principal.p0.id
  permissions  = [each.value.enable_admin ? "ADMIN" : "USER"]
}

resource "p0_databricks_workspace" "this" {
  for_each = local.workspaces

  id                         = each.key
  account                    = p0_databricks_account.this.id
  workspace_url              = each.value.url
  workspace_name             = each.value.name
  is_workspace_admin_enabled = each.value.enable_admin

  depends_on = [databricks_mws_permission_assignment.p0]
}

# ---------------------------------------------------------------------------
# Catalogs (see the p0_databricks_catalog example)
# ---------------------------------------------------------------------------

# databricks_grant manages only this principal's grant. Never use
# databricks_grants on these catalogs: it overwrites every grant on the
# catalog, including the ones P0 hands out.
resource "databricks_grant" "p0_manage" {
  for_each = local.catalogs

  catalog    = each.value.catalog_name
  principal  = databricks_service_principal.p0.application_id
  privileges = ["MANAGE"]

  provider_config {
    workspace_id = each.value.workspace_id
  }

  depends_on = [databricks_mws_permission_assignment.p0]
}

resource "p0_databricks_catalog" "this" {
  for_each = local.catalogs

  workspace_id = p0_databricks_workspace.this[each.value.workspace_id].id
  catalog_name = each.value.catalog_name

  depends_on = [databricks_grant.p0_manage]
}
