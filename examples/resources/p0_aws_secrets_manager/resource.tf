# AWS Secrets Manager requires AWS IAM management installed for the same account first.
# See p0_aws_iam_write for that resource.
resource "p0_aws_secrets_manager" "example" {
  account_id     = p0_aws_iam_write.example.id
  default_region = "us-west-2"
}
