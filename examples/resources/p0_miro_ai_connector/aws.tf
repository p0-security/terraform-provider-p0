# The AWS Lambda variant of the p0_miro_ai_connector example in this
# directory.
#
# Full chain: p0_aws_iam_write (see its example) ->
# p0_miro_ai_connector_staged -> the connector's ECR repository, role and
# Lambda function -> lambda:InvokeFunction for P0's role ->
# p0_miro_ai_connector.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.

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
  miro_ai_aws = {
    # Where P0 invokes the connector.
    account_id       = "123456789012"
    connector_name   = "p0-miro_ai-prod"
    connector_region = "us-east-1"
    # How many days CloudWatch keeps the connector's logs.
    log_retention_days = 30
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. To update, take both from the latest version of this example
    # in the Terraform registry: they change together.
    image_tag    = "sha-7228840"
    image_digest = "sha256:40717d1d6f58958efa7d6467e31650f1733a6e0382368f550a911df19a1a07b1"
  }
}

resource "p0_miro_ai_connector_staged" "aws_example" {
  id = "prod-aws"

  hosting = {
    type             = "aws"
    account_id       = local.miro_ai_aws.account_id
    connector_name   = local.miro_ai_aws.connector_name
    connector_region = local.miro_ai_aws.connector_region
  }

  depends_on = [p0_aws_iam_write.example]
}

# The connector's name as P0 staged it. The resources below take it from here, so
# none is created until staging succeeds.
locals {
  miro_ai_aws_staged = p0_miro_ai_connector_staged.aws_example.hosting
}

# P0 derives the connector's ARN from the account, region and name above, so the
# connector must be deployed exactly there. Add the credential settings your
# other AWS providers use, such as profile or assume_role, if they have any.
provider "aws" {
  alias               = "miro_ai"
  region              = local.miro_ai_aws.connector_region
  allowed_account_ids = [local.miro_ai_aws.account_id]
}

data "docker_registry_image" "miro_ai" {
  name = "p0security/p0-connector-miro-ai:${local.miro_ai_aws.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.miro_ai_aws.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here."
    }
  }
}

resource "aws_ecr_repository" "miro_ai" {
  provider = aws.miro_ai

  name = local.miro_ai_aws_staged.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that runs as the connector.
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
resource "terraform_data" "miro_ai_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.miro_ai.sha256_digest
    repository_url = aws_ecr_repository.miro_ai.repository_url
    tag            = local.miro_ai_aws.image_tag
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
      REGION   = local.miro_ai_aws.connector_region
      REGISTRY = split("/", aws_ecr_repository.miro_ai.repository_url)[0]
      SOURCE   = "p0security/p0-connector-miro-ai@${local.miro_ai_aws.image_digest}"
      TARGET   = "${aws_ecr_repository.miro_ai.repository_url}:${local.miro_ai_aws.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "miro_ai" {
  provider = aws.miro_ai

  repository_name = aws_ecr_repository.miro_ai.name
  image_tag       = "${local.miro_ai_aws.image_tag}-${terraform_data.miro_ai_image.id}"
}

# The identity the connector runs as, named after the connector.
resource "aws_iam_role" "miro_ai" {
  provider = aws.miro_ai

  name = local.miro_ai_aws_staged.connector_name
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
resource "aws_cloudwatch_log_group" "miro_ai" {
  provider = aws.miro_ai

  name              = "/aws/lambda/${local.miro_ai_aws_staged.connector_name}"
  retention_in_days = local.miro_ai_aws.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "miro_ai_logs" {
  provider = aws.miro_ai

  name = "WriteConnectorLogs"
  role = aws_iam_role.miro_ai.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.miro_ai.arn}:*"
    }]
  })
}

# No vpc_config: the connector calls Miro (AI)'s API over the internet.
resource "aws_lambda_function" "miro_ai" {
  provider = aws.miro_ai

  function_name = local.miro_ai_aws_staged.connector_name
  description   = "P0 Miro (AI) connector"
  role          = aws_iam_role.miro_ai.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.miro_ai.repository_url}@${data.aws_ecr_image.miro_ai.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60


  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.miro_ai]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it. The role is the one the p0_aws_iam_write example creates.
resource "aws_iam_role_policy" "miro_ai_invoke" {
  provider = aws.miro_ai

  name = "${local.miro_ai_aws_staged.connector_name}-invoke"
  role = aws_iam_role.p0_iam_manager.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.miro_ai.arn
    }]
  })
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector. IAM changes can take a few seconds to take
# effect, so if the first check is denied access, apply again.
resource "p0_miro_ai_connector" "aws_example" {
  id = p0_miro_ai_connector_staged.aws_example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.miro_ai_aws_staged.type
    account_id       = local.miro_ai_aws_staged.account_id
    connector_name   = aws_lambda_function.miro_ai.function_name
    connector_region = local.miro_ai_aws_staged.connector_region
  }

  depends_on = [aws_iam_role_policy.miro_ai_invoke]
}
