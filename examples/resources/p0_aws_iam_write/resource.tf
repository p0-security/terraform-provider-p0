# Stage the account: computes the role name, trust policy, and inline policy P0 needs to manage IAM.
# GovCloud: set partition = "aws-us-gov" on this staged resource and the p0_aws_iam_write below; the two must match or install fails.
resource "p0_aws_iam_write_staged" "example" {
  id = "123456789012"
}

resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example.role.name
  assume_role_policy = p0_aws_iam_write_staged.example.role.trust_policy
}

# Standalone policy resource: the aws_iam_role inline_policy block was removed in AWS provider v6.
resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example.role.inline_policy
}

# Install only after the role and inline policy exist, or validation fails.
resource "p0_aws_iam_write" "example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [aws_iam_role_policy.p0_iam_manager]

  # Login type. Alternatives to "iam": "idc" (also set parent = IDC account ID + an identity block);
  # "federated" (set a provider block instead of identity -- see the staged example's federated.tf).
  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}
