# Installs P0's Auth0 connector prod on AWS Lambda. Full chain:
# p0_aws_iam_write (see its example) -> p0_auth0_connector_staged -> the
# connector's Lambda function -> p0_auth0_connector -> p0_auth0_tenant.
#
# This example hosts the connector on AWS Lambda. For Google Cloud Run, see
# gcp.tf in this directory.
#
# The connector's Terraform is what P0's Auth0 installer generates for the
# connector, with p0_auth0_connector_staged before it and p0_auth0_connector
# in place of its output. Copy it from the installer, which pins the current
# connector image.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.
#
# One connector serves every Auth0 tenant that names it in P0. Each
# tenant's own Terraform lets this connector read that tenant's
# client secret (see the p0_auth0_tenant example). This configuration
# grants it no secret.

terraform {
  # terraform_data needs Terraform 1.4.
  required_version = ">= 1.4"

  required_providers {
    p0 = {
      source = "p0-security/p0"
    }
    aws = {
      source = "hashicorp/aws"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

locals {
  p0_auth0_connector_prod = {
    # Where P0 invokes the connector. P0 derives the connector's ARN from the
    # account, region and name, so deploy the connector exactly there. P0 fixes
    # these when it adds the connector.
    account_id       = "123456789012"
    connector_name   = "p0-auth0-prod"
    connector_region = "us-west-2"
    # The email domains the connector acts for: a regular expression that the
    # connector tests against the part of an email after the "@". Nothing
    # anchors it for you, so keep the "^" and "$": without them a pattern for
    # acme.com also matches notacme.com. Change it to cover everyone who may be
    # given access, such as "^(acme\\.com|acme\\.io)$" for two domains. The
    # connector won't start without a pattern. P0 can't change it, and shows
    # the one deployed as allowed_domains on each p0_auth0_tenant.
    domain_allow_pattern = "^example\\.com$"
    # How many days CloudWatch keeps the connector's logs.
    log_retention_days = 30
    # The role P0 assumes in this account, which invokes the connector.
    p0_role_name = "P0RoleIamManager"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These are placeholders, for an image that doesn't exist. Use
    # the pinned values in the Terraform that P0's installer shows, or the
    # image's published tag and digest.
    image_tag    = "sha-1234567"
    image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

# Tells P0 where the connector will run, and names the role it must run as.
resource "p0_auth0_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "aws"
    account_id       = local.p0_auth0_connector_prod.account_id
    connector_name   = local.p0_auth0_connector_prod.connector_name
    connector_region = local.p0_auth0_connector_prod.connector_region
  }
}

# Add the credential settings your other AWS providers use, such as profile or
# assume_role, if they have any.
provider "aws" {
  alias               = "p0_auth0_connector_prod"
  region              = local.p0_auth0_connector_prod.connector_region
  allowed_account_ids = [local.p0_auth0_connector_prod.account_id]
}

data "docker_registry_image" "p0_auth0_connector_prod" {
  name = "p0security/p0-connector-auth0:${local.p0_auth0_connector_prod.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_auth0_connector_prod.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Auth0 installer, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "p0_auth0_connector_prod" {
  provider = aws.p0_auth0_connector_prod

  name = local.p0_auth0_connector_prod.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that runs with access to the client secrets.
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Copies the pinned image from Docker Hub into the ECR repository, tagged with
# the image's tag and this copy's id. ECR's tags can't be overwritten, so each
# copy needs a tag of its own: copying the image again, to roll back or after
# an interrupted apply, pushes a new one.
resource "terraform_data" "p0_auth0_connector_prod_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.p0_auth0_connector_prod.sha256_digest
    repository_url = aws_ecr_repository.p0_auth0_connector_prod.repository_url
    tag            = local.p0_auth0_connector_prod.image_tag
  }

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
      REGION   = local.p0_auth0_connector_prod.connector_region
      REGISTRY = split("/", aws_ecr_repository.p0_auth0_connector_prod.repository_url)[0]
      SOURCE   = "p0security/p0-connector-auth0@${local.p0_auth0_connector_prod.image_digest}"
      TARGET   = "${aws_ecr_repository.p0_auth0_connector_prod.repository_url}:${local.p0_auth0_connector_prod.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "p0_auth0_connector_prod" {
  provider = aws.p0_auth0_connector_prod

  repository_name = aws_ecr_repository.p0_auth0_connector_prod.name
  image_tag       = "${local.p0_auth0_connector_prod.image_tag}-${terraform_data.p0_auth0_connector_prod_image.id}"
}

# The connector's role, which P0 names after the connector and expects by
# exactly that name. Each tenant's Terraform adds a policy to it that
# lets the connector read that tenant's client secret.
resource "aws_iam_role" "p0_auth0_connector_prod" {
  provider = aws.p0_auth0_connector_prod

  name = p0_auth0_connector_staged.example.connector_identity.role_name
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
resource "aws_cloudwatch_log_group" "p0_auth0_connector_prod" {
  provider = aws.p0_auth0_connector_prod

  name              = "/aws/lambda/${local.p0_auth0_connector_prod.connector_name}"
  retention_in_days = local.p0_auth0_connector_prod.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "p0_auth0_connector_prod_logs" {
  provider = aws.p0_auth0_connector_prod

  name = "WriteConnectorLogs"
  role = aws_iam_role.p0_auth0_connector_prod.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.p0_auth0_connector_prod.arn}:*"
    }]
  })
}

# No vpc_config: the connector calls the Auth0 tenant over the internet.
resource "aws_lambda_function" "p0_auth0_connector_prod" {
  provider = aws.p0_auth0_connector_prod

  function_name = local.p0_auth0_connector_prod.connector_name
  description   = "P0 Auth0 connector prod"
  role          = aws_iam_role.p0_auth0_connector_prod.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.p0_auth0_connector_prod.repository_url}@${data.aws_ecr_image.p0_auth0_connector_prod.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  environment {
    variables = {
      DOMAIN_ALLOW_PATTERN = local.p0_auth0_connector_prod.domain_allow_pattern
    }
  }

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.p0_auth0_connector_prod]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it.
resource "aws_iam_role_policy" "p0_auth0_connector_prod_invoke" {
  provider = aws.p0_auth0_connector_prod

  name = "${local.p0_auth0_connector_prod.connector_name}-invoke"
  role = local.p0_auth0_connector_prod.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.p0_auth0_connector_prod.arn
    }]
  })
}

# Completes the install. Creating it has P0 check that it can invoke the
# connector, so it waits until P0's role may. IAM changes can take a few seconds
# to propagate, so if the first check is denied access, apply again.
resource "p0_auth0_connector" "example" {
  id      = p0_auth0_connector_staged.example.id
  hosting = p0_auth0_connector_staged.example.hosting

  depends_on = [aws_iam_role_policy.p0_auth0_connector_prod_invoke]
}
