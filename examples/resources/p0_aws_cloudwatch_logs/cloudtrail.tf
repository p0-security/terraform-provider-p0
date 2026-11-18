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
