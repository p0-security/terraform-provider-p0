resource "aws_sqs_queue" "p0_audit_events" {
  name = "p0-audit-events"
}

resource "p0_aws_sqs_audit_logs_staged" "example" {
  id = aws_sqs_queue.p0_audit_events.arn
}

# The role P0 assumes to send to the queue. Its trust policy and inline policy must
# match these exactly, or P0's check fails.
resource "aws_iam_role" "p0_event_sender" {
  name = "P0RoleIamEventSender"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = "accounts.google.com" }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "accounts.google.com:aud" = p0_aws_sqs_audit_logs_staged.example.service_account_id
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "p0_event_sender" {
  name = "P0RoleIamEventSenderPolicy"
  role = aws_iam_role.p0_event_sender.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "P0CanVerifyOwnRole"
        Effect   = "Allow"
        Action   = ["iam:GetRole", "iam:GetRolePolicy"]
        Resource = aws_iam_role.p0_event_sender.arn
      },
      {
        Sid      = "P0CanSendToQueue"
        Effect   = "Allow"
        Action   = "sqs:SendMessage"
        Resource = aws_sqs_queue.p0_audit_events.arn
      },
      # Add for queues encrypted with a customer managed KMS key, and set
      # kms_key_arn on both P0 resources:
      # {
      #   Sid      = "P0CanEncryptForQueue"
      #   Effect   = "Allow"
      #   Action   = ["kms:GenerateDataKey", "kms:Decrypt"]
      #   Resource = "<kms key arn>"
      # },
    ]
  })
}

# Checks the role and sends one test message to the queue.
resource "p0_aws_sqs_audit_logs" "example" {
  id         = p0_aws_sqs_audit_logs_staged.example.id
  depends_on = [aws_iam_role_policy.p0_event_sender]
}
