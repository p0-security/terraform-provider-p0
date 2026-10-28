data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

resource "aws_s3_bucket" "p0_audit_logs" {
  bucket = "p0-audit-logs"
}

resource "p0_aws_s3_audit_logs_staged" "example" {
  id         = aws_s3_bucket.p0_audit_logs.bucket
  account_id = data.aws_caller_identity.current.account_id
  region     = data.aws_region.current.name
}

# The role P0 assumes to write to the bucket. Its trust policy and inline policy
# must match these exactly, or P0's check fails.
resource "aws_iam_role" "p0_s3_writer" {
  name = "P0RoleIamS3Writer"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = "accounts.google.com" }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "accounts.google.com:aud" = p0_aws_s3_audit_logs_staged.example.service_account_id
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "p0_s3_writer" {
  name = "P0RoleIamS3WriterPolicy"
  role = aws_iam_role.p0_s3_writer.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "P0CanVerifyOwnRole"
        Effect   = "Allow"
        Action   = ["iam:GetRole", "iam:GetRolePolicy"]
        Resource = aws_iam_role.p0_s3_writer.arn
      },
      {
        Sid      = "P0CanWriteAuditFiles"
        Effect   = "Allow"
        Action   = "s3:PutObject"
        Resource = "${aws_s3_bucket.p0_audit_logs.arn}/*"
      },
    ]
  })
}

# Checks the role and writes one test file to p0-install-check/ in the bucket.
resource "p0_aws_s3_audit_logs" "example" {
  id         = p0_aws_s3_audit_logs_staged.example.id
  account_id = p0_aws_s3_audit_logs_staged.example.account_id
  region     = p0_aws_s3_audit_logs_staged.example.region
  depends_on = [aws_iam_role_policy.p0_s3_writer]
}
