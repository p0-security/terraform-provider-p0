# Stages an SQS queue for P0 audit logs. P0's AWS integration must already be
# installed for the queue's account.
resource "p0_aws_sqs_audit_logs_staged" "example" {
  id = "arn:aws:sqs:us-west-2:123456789012:p0-audit-events"

  # Only for queues encrypted with a customer managed KMS key.
  # kms_key_arn = "arn:aws:kms:us-west-2:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab"
}
