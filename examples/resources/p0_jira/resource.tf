# Installs Jira for the site acme.atlassian.net, with P0's Jira connector on AWS
# Lambda and the service account's OAuth client secret in AWS Secrets Manager.
# For Google Cloud Run, see gcp.tf in this directory.
#
# Before you apply this, create the site's Atlassian service account and its
# OAuth credential, store the credential's client secret in Secrets Manager as
# p0/jira/acme, and install p0_aws_iam_write for the account the connector runs
# in. This resource's page lists what the service account needs. The example
# doesn't create the secret, which keeps the client secret out of Terraform's
# state.
#
# Creating p0_jira has the connector read the client secret, so store the
# client secret before you apply. To store it, with the AWS CLI signed in to the secret's
# account:
#
#   aws secretsmanager create-secret \
#     --name p0/jira/acme \
#     --secret-string file://client-secret.txt --region us-east-1
#
# client-secret.txt holds the credential's client secret, and us-east-1 is the
# secret's region, secrets_region below. To encrypt the secret with a
# customer-managed KMS key, which a secret in another account needs, add
# --kms-key-id with the key's ARN to create-secret.
#
# The connector's Terraform follows what P0's Jira installer generates for the
# site, with p0_jira in place of its output, and names the secret by its account
# and name where the installer uses a partial ARN. The installer pins the
# current connector image.

# P0's Jira connector for the site acme.atlassian.net, on AWS Lambda.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.
#
# Other sites can share this configuration, but each needs its own connector
# name: the connector's role, repository and log group are named after it, and
# these names are unique in an AWS account.
#
# The connector reads the site's client secret from AWS Secrets Manager in
# this account. It needs no secret to exist yet: until the secret does, the
# connector just has nothing to read.
# This doesn't create the secret, and never reads or holds its value.

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
      version = ">= 3.3, < 5.0"
    }
  }
}

locals {
  p0_jira_site_acme = {
    # Where P0 invokes the connector. P0's installer has the same values.
    account_id       = "123456789012"
    connector_name   = "p0-jira-site-acme"
    connector_region = "us-west-2"
    # The connector gives Jira access only to users whose whole email domain
    # matches this. It's also p0_jira's domain_pattern, which P0 checks against
    # the connector's. A new pattern replaces the site in P0.
    domain_pattern = "(.+\\.)?example\\.com"
    # How many days CloudWatch keeps the connector's logs. Change it if you
    # need to.
    log_retention_days = 30
    # The role P0 assumes in this account, which invokes the connector.
    p0_role_name = "P0RoleIamManager"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These are placeholders, for an image that doesn't exist. Use
    # the pinned values in the Terraform that P0's installer shows, or the
    # image's published tag and digest.
    image_tag    = "sha-1234567"
    image_digest = "sha256:abababababababababababababababababababababababababababababababab"
    # The secret that holds the client secret: its name, and the account and
    # region it's in. A new secret replaces the site in P0. To rotate the
    # client secret, add a new version to this secret instead.
    client_secret_name = "p0/jira/acme"
    secrets_account_id = "123456789012"
    secrets_region     = "us-east-1"
    # The ARN of the KMS key that encrypts the secret, if that's a
    # customer-managed key: the key's own ARN, not an alias. The default
    # aws/secretsmanager key needs nothing for a secret in this account.
    kms_key_arn = null
  }
}

# P0 derives the connector's ARN from the account, region and name above, so the
# connector must be deployed exactly there. Add the credential settings your
# other AWS providers use, such as profile or assume_role, if they have any.
provider "aws" {
  alias               = "p0_jira_site_acme"
  region              = local.p0_jira_site_acme.connector_region
  allowed_account_ids = [local.p0_jira_site_acme.account_id]
}

data "aws_partition" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme
}

# Stops the plan if the tag has been moved to another image since P0 pinned
# it, so that only the image P0 pinned is deployed.
data "docker_registry_image" "p0_jira_site_acme" {
  name = "p0security/p0-connector-jira:${local.p0_jira_site_acme.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_jira_site_acme.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Jira installer, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name = local.p0_jira_site_acme.connector_name
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
resource "terraform_data" "p0_jira_site_acme_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.p0_jira_site_acme.sha256_digest
    repository_url = aws_ecr_repository.p0_jira_site_acme.repository_url
    tag            = local.p0_jira_site_acme.image_tag
  }

  provisioner "local-exec" {
    # Signs Docker in to ECR in a configuration directory of its own, which it
    # deletes at the end. So the login doesn't outlast the copy, connectors
    # copied at the same time don't share one, and your own Docker logins are
    # left alone. The Docker daemon is still the one your Docker context
    # points at.
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
      REGION   = local.p0_jira_site_acme.connector_region
      REGISTRY = split("/", aws_ecr_repository.p0_jira_site_acme.repository_url)[0]
      SOURCE   = "p0security/p0-connector-jira@${local.p0_jira_site_acme.image_digest}"
      TARGET   = "${aws_ecr_repository.p0_jira_site_acme.repository_url}:${local.p0_jira_site_acme.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  repository_name = aws_ecr_repository.p0_jira_site_acme.name
  image_tag       = "${local.p0_jira_site_acme.image_tag}-${terraform_data.p0_jira_site_acme_image.id}"
}

# The identity the connector runs as, named after the connector.
resource "aws_iam_role" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name = local.p0_jira_site_acme.connector_name
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
resource "aws_cloudwatch_log_group" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name              = "/aws/lambda/${local.p0_jira_site_acme.connector_name}"
  retention_in_days = local.p0_jira_site_acme.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "p0_jira_site_acme_logs" {
  provider = aws.p0_jira_site_acme

  name = "WriteConnectorLogs"
  role = aws_iam_role.p0_jira_site_acme.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.p0_jira_site_acme.arn}:*"
    }]
  })
}

# The connector reads the site's client secret, and nothing else.
resource "aws_iam_role_policy" "p0_jira_site_acme_read" {
  provider = aws.p0_jira_site_acme

  name = "ReadJiraClientSecret"
  role = aws_iam_role.p0_jira_site_acme.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadSecret"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # "??????" matches the six random characters that end the secret's
        # ARN, so the connector gets this one secret and no other.
        Resource = "arn:aws:secretsmanager:${local.p0_jira_site_acme.secrets_region}:${local.p0_jira_site_acme.secrets_account_id}:secret:${local.p0_jira_site_acme.client_secret_name}-??????"
      }],
      local.p0_jira_site_acme.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_jira_site_acme.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_jira_site_acme.secrets_region}.${data.aws_partition.p0_jira_site_acme.dns_suffix}"
          }
        }
      }],
    )
  })

  lifecycle {
    # A key in the secret's region, by its ARN. It may be in another account.
    precondition {
      condition     = local.p0_jira_site_acme.kms_key_arn == null || can(regex("^arn:aws:kms:${local.p0_jira_site_acme.secrets_region}:[0-9]{12}:key/(mrk-)?[0-9a-f-]+$", local.p0_jira_site_acme.kms_key_arn))
      error_message = "Set kms_key_arn to the ARN of a KMS key in the secret's region, such as arn:aws:kms:<region>:<account>:key/<key id>. An alias, or a key's ID alone, won't do: the policy needs the key's ARN."
    }
  }
}

# No vpc_config: the connector calls Atlassian over the internet.
resource "aws_lambda_function" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  function_name = local.p0_jira_site_acme.connector_name
  description   = "P0 Jira connector for acme.atlassian.net"
  role          = aws_iam_role.p0_jira_site_acme.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.p0_jira_site_acme.repository_url}@${data.aws_ecr_image.p0_jira_site_acme.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  environment {
    variables = {
      DOMAIN_PATTERN = local.p0_jira_site_acme.domain_pattern
    }
  }

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.p0_jira_site_acme]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it.
resource "aws_iam_role_policy" "p0_jira_site_acme_invoke" {
  provider = aws.p0_jira_site_acme

  name = "${local.p0_jira_site_acme.connector_name}-invoke"
  role = local.p0_jira_site_acme.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.p0_jira_site_acme.arn
    }]
  })
}

# Installs Jira for the site. Creating it has P0 check the site through the
# connector, so it waits until P0 can invoke the connector and the connector
# can read the client secret. IAM changes can take a few seconds to take
# effect, so if the first check is denied access, apply again.
resource "p0_jira" "example" {
  site = "acme.atlassian.net"

  hosting = {
    type             = "aws"
    account_id       = local.p0_jira_site_acme.account_id
    connector_name   = aws_lambda_function.p0_jira_site_acme.function_name
    connector_region = local.p0_jira_site_acme.connector_region
  }

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_jira_site_acme.secrets_account_id
    secrets_region = local.p0_jira_site_acme.secrets_region
  }
  client_secret_name = local.p0_jira_site_acme.client_secret_name
  domain_pattern     = local.p0_jira_site_acme.domain_pattern

  # The client ID of the service account's OAuth credential. A new credential
  # updates the site in place.
  client_id = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"

  depends_on = [
    aws_iam_role_policy.p0_jira_site_acme_invoke,
    aws_iam_role_policy.p0_jira_site_acme_read,
  ]
}
