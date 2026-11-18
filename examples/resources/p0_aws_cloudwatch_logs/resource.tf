# Installs an AWS CloudWatch Logs log source: P0's connector in one AWS account
# and region, which reads CloudTrail and RDS logs from CloudWatch Logs so that
# Access Logs can show what each request's access was used for. The connector
# is a Lambda function outside any VPC. P0 invokes it through a role of its own,
# which can do nothing else.
#
# Full chain: p0_aws_cloudwatch_logs_staged -> the connector's ECR repository,
# roles and Lambda function, and P0's invoke role -> p0_aws_cloudwatch_logs.
#
# cloudtrail.tf in this directory creates an organization trail that delivers to
# the log group named below. It's your own infrastructure, not P0's: if a trail
# already delivers to CloudWatch Logs in this account and region, name its log
# group below and delete cloudtrail.tf.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to the account below.

terraform {
  # terraform_data needs Terraform 1.4.
  required_version = ">= 1.4"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 3.3, < 5.0"
    }
    # 0.98.0 added the log-source resources.
    p0 = {
      source  = "p0-security/p0"
      version = ">= 0.98.0"
    }
  }
}

locals {
  # The AWS account and region whose CloudWatch Logs P0 reads, and where the
  # connector runs. P0 allows one log source per account and region.
  account_id = "123456789012"
  region     = "us-west-2"

  # The CloudWatch Logs group that your CloudTrail trail delivers to, in this
  # account and region.
  cloudtrail_log_group_name = "aws-cloudtrail-logs"

  # How many days CloudWatch keeps the connector's own logs.
  log_retention_days = 30

  # P0's connector image: its tag on Docker Hub, and the digest that tag must
  # resolve to. To update, take both from the latest version of this example in
  # the Terraform registry: they change together.
  connector_image_name   = "p0security/p0-connector-session-logging-aws"
  connector_image_tag    = "sha-a266a1e"
  connector_image_digest = "sha256:a11287d7fd01e354b3c405cc0ab21990647e9f6cbedefa7a8ab813c5743002cb"
}

# P0 invokes the connector by its name in the account and region above, so it
# must be deployed exactly there. Add the credential settings your other AWS
# providers use, such as profile or assume_role, if they have any.
provider "aws" {
  region              = local.region
  allowed_account_ids = [local.account_id]
}

# Staging records the log source in P0, which answers with the connector's name,
# P0's invoke role and the policies below.
resource "p0_aws_cloudwatch_logs_staged" "example" {
  id         = "${local.account_id}:${local.region}"
  account_id = local.account_id
  region     = local.region

  # Leave cloudtrail out if no trail delivers to CloudWatch Logs here: P0 then
  # reads only RDS logs. Use coverage = "account" for a trail that logs only
  # this account.
  cloudtrail = {
    log_group_name = local.cloudtrail_log_group_name
    coverage       = "organization"
  }
}

# What P0 staged. The resources below take it from here, so none is created
# until staging succeeds.
locals {
  log_source = p0_aws_cloudwatch_logs_staged.example
}

# Stops the plan if the tag has been moved to another image since P0 pinned it,
# so that only the image P0 pinned is deployed.
data "docker_registry_image" "connector" {
  name = "${local.connector_image_name}:${local.connector_image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.connector_image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Take both from the latest p0_aws_cloudwatch_logs example in P0's Terraform registry docs, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "connector" {
  name = local.log_source.connector_name
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
# copy needs a tag of its own.
resource "terraform_data" "connector_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.connector.sha256_digest
    repository_url = aws_ecr_repository.connector.repository_url
    tag            = local.connector_image_tag
  }

  provisioner "local-exec" {
    # Signs Docker in to ECR in a configuration directory of its own, which it
    # deletes at the end, so your own Docker logins are left alone.
    command = <<-EOT
      set -e
      if [ -z "$DOCKER_HOST" ]; then
        DOCKER_HOST="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
        export DOCKER_HOST
      fi
      DOCKER_CONFIG="$(mktemp -d)"
      export DOCKER_CONFIG
      trap 'rm -rf "$DOCKER_CONFIG"' EXIT
      aws ecr get-login-password --region "$REGION" |
        docker login --username AWS --password-stdin "$REGISTRY"
      docker pull --platform linux/amd64 "$SOURCE"
      docker tag "$SOURCE" "$TARGET"
      docker push "$TARGET"
    EOT

    environment = {
      REGION   = local.region
      REGISTRY = split("/", aws_ecr_repository.connector.repository_url)[0]
      SOURCE   = "${local.connector_image_name}@${local.connector_image_digest}"
      TARGET   = "${aws_ecr_repository.connector.repository_url}:${local.connector_image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, so
# the digest there can differ from the one pinned above. Lambda needs ECR's.
data "aws_ecr_image" "connector" {
  repository_name = aws_ecr_repository.connector.name
  image_tag       = "${local.connector_image_tag}-${terraform_data.connector_image.id}"
}

# The identity the connector runs as. IAM role names are global to an account,
# so it carries the region.
resource "aws_iam_role" "connector" {
  name = "${local.log_source.connector_name}-${local.region}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

# The log group Lambda writes the connector's own logs to.
resource "aws_cloudwatch_log_group" "connector" {
  name              = "/aws/lambda/${local.log_source.connector_name}"
  retention_in_days = local.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "connector_logs" {
  name = "WriteConnectorLogs"
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

# What the connector reads: Logs Insights on your CloudTrail log group and on
# RDS's log groups in this account and region, and your trails' settings.
resource "aws_iam_role_policy" "connector_read" {
  name   = "ReadAccessLogs"
  role   = aws_iam_role.connector.id
  policy = local.log_source.execution_policy
}

# No vpc_config: the connector calls only AWS's public APIs.
resource "aws_lambda_function" "connector" {
  function_name = local.log_source.connector_name
  description   = "P0 AWS CloudWatch Logs connector"
  role          = aws_iam_role.connector.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.connector.repository_url}@${data.aws_ecr_image.connector.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  # The install check waits up to a minute for a sample Logs Insights query.
  timeout = 120

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.connector]
}

# The role P0 assumes to invoke the connector. Its trust policy lets P0's own
# service account, and no other, assume it.
resource "aws_iam_role" "p0_invoke" {
  name               = local.log_source.invoke_role_name
  assume_role_policy = local.log_source.invoke_trust_policy
}

# P0's invoke role can invoke the connector, and nothing else.
resource "aws_iam_role_policy" "p0_invoke" {
  name = "InvokeConnector"
  role = aws_iam_role.p0_invoke.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.connector.arn
    }]
  })
}

# Completes the install. P0 checks, through the connector, that the CloudTrail
# log group holds an event from the last 24 hours and that the trail's coverage
# matches. A new trail takes a few minutes to deliver its first events, and a
# new IAM role a few seconds to take effect: if the first apply fails, apply
# again.
resource "p0_aws_cloudwatch_logs" "example" {
  id         = local.log_source.id
  account_id = local.log_source.account_id
  region     = local.log_source.region
  cloudtrail = local.log_source.cloudtrail

  depends_on = [
    aws_iam_role_policy.connector_logs,
    aws_iam_role_policy.connector_read,
    aws_iam_role_policy.p0_invoke,
  ]
}
