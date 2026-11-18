---
page_title: "p0_aws_cloudwatch_logs Resource - p0"
subcategory: ""
description: |-
  An AWS CloudWatch Logs log source installation: one AWS account and region whose CloudTrail and RDS logs P0 reads, so that Access Logs can show what each request's access was used for.
  Important: Before creating this resource you must stage the log source with p0_aws_cloudwatch_logs_staged, and deploy the following in the staged account and region, with the names and policies that staging returns:
  The connector's Lambda function, named connector_name, from P0's pinned connector image copied into ECR. It runs outside any VPC.The connector's execution role, with execution_policy.P0's invoke role, named invoke_role_name, with invoke_trust_policy and a policy that allows lambda:InvokeFunction on the connector and nothing else.
  If cloudtrail is set, its trail must already deliver to the log group: creating this resource has P0 check, through the connector, that the log group holds a CloudTrail event from the last 24 hours and that the trail's coverage matches. P0 retries for about 15 seconds while new IAM roles and the function take effect, then the apply fails with P0's message. If the first apply is denied access, apply again.
  A log source that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.
  Note: Access Logs is in beta. P0 offers this resource only to organizations that have Access Logs turned on.
---

# p0_aws_cloudwatch_logs (Resource)

An AWS CloudWatch Logs log source installation: one AWS account and region whose CloudTrail and RDS logs P0 reads, so that Access Logs can show what each request's access was used for.

**Important:** Before creating this resource you must stage the log source with `p0_aws_cloudwatch_logs_staged`, and deploy the following in the staged account and region, with the names and policies that staging returns:

- The connector's Lambda function, named `connector_name`, from P0's pinned connector image copied into ECR. It runs outside any VPC.
- The connector's execution role, with `execution_policy`.
- P0's invoke role, named `invoke_role_name`, with `invoke_trust_policy` and a policy that allows `lambda:InvokeFunction` on the connector and nothing else.

If `cloudtrail` is set, its trail must already deliver to the log group: creating this resource has P0 check, through the connector, that the log group holds a CloudTrail event from the last 24 hours and that the trail's coverage matches. P0 retries for about 15 seconds while new IAM roles and the function take effect, then the apply fails with P0's message. If the first apply is denied access, apply again.

A log source that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

**Note:** Access Logs is in beta. P0 offers this resource only to organizations that have Access Logs turned on.

## Example Usage

```terraform
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
```

### CloudTrail organization trail

Your own infrastructure, not P0's: an organization trail that delivers to the log group above. Skip it if a trail already delivers to CloudWatch Logs in the log source's account and region.

```terraform
# YOUR OWN INFRASTRUCTURE, NOT P0'S. An example organization trail that delivers
# every account's CloudTrail events in your AWS organization to CloudWatch Logs,
# where the p0_aws_cloudwatch_logs connector in resource.tf reads them. P0
# doesn't manage, check or need anything here beyond the log group the trail
# delivers to. Adapt it to your own logging standards, or delete this file if a
# trail already delivers to CloudWatch Logs.
#
# An organization trail is created in your organization's management account,
# or in a delegated CloudTrail administrator account, and its log group lives
# there. So the log source in resource.tf names that account and the trail's
# home region. Trusted access for CloudTrail must be turned on in AWS
# Organizations.
#
# CloudTrail charges for the trail's S3 delivery beyond the first copy of
# management events, and CloudWatch Logs charges to ingest and keep the events.
# Logs Insights charges for the data each Access Logs read scans.

data "aws_organizations_organization" "current" {}

locals {
  cloudtrail_name = "organization-trail"
  cloudtrail_arn  = "arn:aws:cloudtrail:${local.region}:${local.account_id}:trail/${local.cloudtrail_name}"
}

# The log group that resource.tf names, and that P0's connector reads.
resource "aws_cloudwatch_log_group" "cloudtrail" {
  name              = local.cloudtrail_log_group_name
  retention_in_days = 90
}

# The role CloudTrail delivers to the log group as.
resource "aws_iam_role" "cloudtrail_delivery" {
  name = "${local.cloudtrail_name}-cloudwatch-delivery"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "cloudtrail.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = { StringEquals = { "aws:SourceArn" = local.cloudtrail_arn } }
    }]
  })
}

# An organization trail writes each account's events to a stream of its own, in
# the management account's log group.
resource "aws_iam_role_policy" "cloudtrail_delivery" {
  name = "DeliverToCloudWatchLogs"
  role = aws_iam_role.cloudtrail_delivery.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.cloudtrail.arn}:log-stream:*"
    }]
  })
}

# Every trail also delivers to S3.
resource "aws_s3_bucket" "cloudtrail" {
  bucket_prefix = "${local.cloudtrail_name}-"
  # Lets terraform destroy remove the bucket with the trail's files in it.
  # Remove it to keep them.
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "cloudtrail" {
  bucket                  = aws_s3_bucket.cloudtrail.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_policy" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "AWSCloudTrailAclCheck"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:GetBucketAcl"
        Resource  = aws_s3_bucket.cloudtrail.arn
        Condition = { StringEquals = { "aws:SourceArn" = local.cloudtrail_arn } }
      },
      {
        Sid       = "AWSCloudTrailWrite"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource = [
          "${aws_s3_bucket.cloudtrail.arn}/AWSLogs/${local.account_id}/*",
          "${aws_s3_bucket.cloudtrail.arn}/AWSLogs/${data.aws_organizations_organization.current.id}/*",
        ]
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"  = "bucket-owner-full-control"
            "aws:SourceArn" = local.cloudtrail_arn
          }
        }
      },
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.cloudtrail]
}

# Logs every region's management events, in every account of your organization.
# Access Logs shows reads and writes of data, such as S3 objects, only if the
# trail also logs data events for them: add an event_selector or
# advanced_event_selector for the resource types you want. Data events can be
# many, and CloudTrail and CloudWatch Logs charge for each.
resource "aws_cloudtrail" "organization" {
  name                          = local.cloudtrail_name
  s3_bucket_name                = aws_s3_bucket.cloudtrail.id
  is_organization_trail         = true
  is_multi_region_trail         = true
  include_global_service_events = true
  enable_log_file_validation    = true

  cloud_watch_logs_group_arn = "${aws_cloudwatch_log_group.cloudtrail.arn}:*"
  cloud_watch_logs_role_arn  = aws_iam_role.cloudtrail_delivery.arn

  depends_on = [
    aws_s3_bucket_policy.cloudtrail,
    aws_iam_role_policy.cloudtrail_delivery,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `account_id` (String) The AWS account whose CloudWatch Logs P0 reads, and where the connector runs
- `id` (String) The log source's name in P0. Any name works; `<account ID>:<region>`, such as `123456789012:us-west-2`, keeps it unique, since P0 allows one log source per account and region.
- `region` (String) The AWS region whose CloudWatch Logs P0 reads, and where the connector runs. P0 supports only commercial AWS regions.

### Optional

- `cloudtrail` (Attributes) The CloudTrail trail that delivers to CloudWatch Logs in this account and region. Leave it unset if there is none: P0 then reads only RDS logs here. P0 fixes the trail when it stages the log source, since the connector's read policy names its log group, so changing it replaces the log source. See `cloudtrail.tf` in the `p0_aws_cloudwatch_logs` examples for an organization trail that delivers to CloudWatch Logs. (see [below for nested schema](#nestedatt--cloudtrail))

### Read-Only

- `connector_name` (String) The name of the connector's Lambda function, which P0 invokes. The example names the connector's ECR repository and log group after it, and its execution role `<connector_name>-<region>`.
- `execution_policy` (String) The policy the connector's execution role needs, as JSON: Logs Insights on the CloudTrail log group and on RDS's log groups in this account and region, and reading the trails' settings
- `invoke_role_name` (String) The name of the IAM role P0 assumes to invoke the connector, and nothing else
- `invoke_trust_policy` (String) The trust policy of the invoke role, as JSON. It lets P0's own service account, and no other, assume the role.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--cloudtrail"></a>
### Nested Schema for `cloudtrail`

Required:

- `coverage` (String) Which accounts the trail logs: `account` for a trail that logs this AWS account's events, or `organization` for an organization trail that logs every account in your AWS organization. P0 allows one `organization` log source, and one `account` log source per account.
- `log_group_name` (String) The CloudWatch Logs group in this account and region that the trail delivers to, such as `aws-cloudtrail-logs`
