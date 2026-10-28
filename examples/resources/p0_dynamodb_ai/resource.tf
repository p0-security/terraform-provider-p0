# Install AWS IAM management (p0_aws_iam_write) and AWS OIDC federation
# (p0_aws_oidc_identity) for the account first. This item adds nothing in AWS.
resource "p0_dynamodb_ai" "example" {
  id = "123456789012"
  depends_on = [
    p0_aws_iam_write.example,
    p0_aws_oidc_identity.example,
  ]
}
