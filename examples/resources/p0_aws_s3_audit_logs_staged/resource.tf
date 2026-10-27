# Stages an S3 bucket for P0 audit logs. P0's AWS integration must already be
# installed for the bucket's account.
resource "p0_aws_s3_audit_logs_staged" "example" {
  id         = "p0-audit-logs"
  account_id = "123456789012"
  region     = "us-west-2"
}
