# Installs GitHub Repositories for the GitHub organization my-github-org, with
# P0's GitHub Repositories connector on AWS Lambda and the GitHub App's private
# key in AWS Secrets Manager. For Google Cloud, see gcp.tf in this directory.
#
# Before you apply this, create the organization's GitHub App, store its private
# key in Secrets Manager as github/my-github-org/private-key, and install
# p0_aws_iam_write for the account the connector runs in. This resource's page
# lists what the App needs. The example doesn't create the key's secret, which
# keeps the key out of Terraform's state.
#
# The connector's Terraform is what P0's GitHub Repositories installer generates
# for the organization, with p0_github_repositories in place of its output. Copy
# it from the installer, which pins the current connector image.

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
    # For gcp.tf, the Google Cloud variant in this directory: its Cloud Run
    # service's deletion_protection needs the google provider 6.0 or later.
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0"
    }
  }
}

# P0's GitHub Repositories connector for my-github-org, on AWS Lambda.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.
#
# P0 runs one connector for each organization, so apply this once, in one
# cloud. Other organizations' connectors can share this configuration, but each
# needs its own connector name: the connector's role, repository and log group
# are named after it, and these names are unique in an AWS account.

locals {
  p0_github_repositories_my_github_org = {
    # Where P0 invokes the connector. P0's installer has the same values.
    account_id       = "123456789012"
    connector_name   = "p0-github-my-github-org"
    connector_region = "us-east-1"
    # The name of the secret, in this account, that holds the GitHub App's
    # private key. The connector can read this secret and no other. A new name
    # replaces the installation. To rotate the key, add a new version to this
    # secret instead.
    private_key_secret_name = "github/my-github-org/private-key"
    secrets_region          = "us-east-1"
    # The ARN of the KMS key that encrypts the secret, if that's a
    # customer-managed key. The default aws/secretsmanager key needs nothing
    # for a secret in this account.
    kms_key_arn = null
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

# P0 derives the connector's ARN from the account, region and name above, so the
# connector must be deployed exactly there. Add the credential settings your
# other AWS providers use, such as profile or assume_role, if they have any.
provider "aws" {
  alias               = "p0_github_repositories_my_github_org"
  region              = local.p0_github_repositories_my_github_org.connector_region
  allowed_account_ids = [local.p0_github_repositories_my_github_org.account_id]
}

data "aws_partition" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org
}

data "docker_registry_image" "p0_github_repositories_my_github_org" {
  name = "p0security/p0-connector-github-repositories:${local.p0_github_repositories_my_github_org.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_github_repositories_my_github_org.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's GitHub Repositories installer, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org

  name = local.p0_github_repositories_my_github_org.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that runs with access to the key.
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
resource "terraform_data" "p0_github_repositories_my_github_org_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.p0_github_repositories_my_github_org.sha256_digest
    repository_url = aws_ecr_repository.p0_github_repositories_my_github_org.repository_url
    tag            = local.p0_github_repositories_my_github_org.image_tag
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
      REGION   = local.p0_github_repositories_my_github_org.connector_region
      REGISTRY = split("/", aws_ecr_repository.p0_github_repositories_my_github_org.repository_url)[0]
      SOURCE   = "p0security/p0-connector-github-repositories@${local.p0_github_repositories_my_github_org.image_digest}"
      TARGET   = "${aws_ecr_repository.p0_github_repositories_my_github_org.repository_url}:${local.p0_github_repositories_my_github_org.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org

  repository_name = aws_ecr_repository.p0_github_repositories_my_github_org.name
  image_tag       = "${local.p0_github_repositories_my_github_org.image_tag}-${terraform_data.p0_github_repositories_my_github_org_image.id}"
}

resource "aws_iam_role" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org

  name = local.p0_github_repositories_my_github_org.connector_name
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
resource "aws_cloudwatch_log_group" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org

  name              = "/aws/lambda/${local.p0_github_repositories_my_github_org.connector_name}"
  retention_in_days = local.p0_github_repositories_my_github_org.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "p0_github_repositories_my_github_org_logs" {
  provider = aws.p0_github_repositories_my_github_org

  name = "WriteConnectorLogs"
  role = aws_iam_role.p0_github_repositories_my_github_org.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.p0_github_repositories_my_github_org.arn}:*"
    }]
  })
}

# The connector reads the GitHub App's private key, and nothing else.
resource "aws_iam_role_policy" "p0_github_repositories_my_github_org_private_key" {
  provider = aws.p0_github_repositories_my_github_org

  name = "ReadGitHubAppPrivateKey"
  role = aws_iam_role.p0_github_repositories_my_github_org.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadPrivateKey"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # A secret's ARN ends in a hyphen and six random characters. "??????"
        # matches those, so the name gets this one secret and no other.
        Resource = join(":", [
          "arn",
          data.aws_partition.p0_github_repositories_my_github_org.partition,
          "secretsmanager",
          local.p0_github_repositories_my_github_org.secrets_region,
          local.p0_github_repositories_my_github_org.account_id,
          "secret",
          "${local.p0_github_repositories_my_github_org.private_key_secret_name}-??????",
        ])
      }],
      local.p0_github_repositories_my_github_org.kms_key_arn == null ? [] : [{
        Sid      = "DecryptPrivateKey"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_github_repositories_my_github_org.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_github_repositories_my_github_org.secrets_region}.${data.aws_partition.p0_github_repositories_my_github_org.dns_suffix}"
          }
        }
      }],
    )
  })
}

# No vpc_config: the connector calls api.github.com over the internet.
resource "aws_lambda_function" "p0_github_repositories_my_github_org" {
  provider = aws.p0_github_repositories_my_github_org

  function_name = local.p0_github_repositories_my_github_org.connector_name
  description   = "P0 GitHub Repositories connector for my-github-org"
  role          = aws_iam_role.p0_github_repositories_my_github_org.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.p0_github_repositories_my_github_org.repository_url}@${data.aws_ecr_image.p0_github_repositories_my_github_org.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.p0_github_repositories_my_github_org]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it.
resource "aws_iam_role_policy" "p0_github_repositories_my_github_org_invoke" {
  provider = aws.p0_github_repositories_my_github_org

  name = "${local.p0_github_repositories_my_github_org.connector_name}-invoke"
  role = local.p0_github_repositories_my_github_org.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.p0_github_repositories_my_github_org.arn
    }]
  })
}

# Installs GitHub Repositories for the organization. Creating it has P0 check
# the install through the connector, so it waits until P0 can invoke the
# connector and the connector can read the key. IAM changes can take a few
# seconds to take effect, so if the first check is denied access, apply again.
resource "p0_github_repositories" "example" {
  org = "my-github-org"
  # Each organization has its own GitHub App, so each installation sets its own
  # app_id.
  app_id = "123456"

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_github_repositories_my_github_org.account_id
    secrets_region = local.p0_github_repositories_my_github_org.secrets_region
  }
  private_key_secret_name = local.p0_github_repositories_my_github_org.private_key_secret_name

  hosting = {
    type             = "aws"
    account_id       = local.p0_github_repositories_my_github_org.account_id
    connector_name   = aws_lambda_function.p0_github_repositories_my_github_org.function_name
    connector_region = local.p0_github_repositories_my_github_org.connector_region
  }

  depends_on = [
    aws_iam_role_policy.p0_github_repositories_my_github_org_invoke,
    aws_iam_role_policy.p0_github_repositories_my_github_org_private_key,
  ]
}
