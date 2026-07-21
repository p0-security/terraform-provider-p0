# Stage the AWS account. This computes the role name, trust policy, and inline
# policy that P0 requires to manage IAM in the account.
#
# GovCloud note: for an aws-us-gov account you must set `partition =
# "aws-us-gov"` on BOTH this staged resource and the p0_aws_iam_write resource
# below. The two partitions must match, or the install will fail.
resource "p0_aws_iam_write_staged" "example" {
  id = "123456789012"
}

# The IAM role that P0 assumes to manage IAM grants in your account. Its name and
# trust policy come from the staged resource's computed outputs.
resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example.role.name
  assume_role_policy = p0_aws_iam_write_staged.example.role.trust_policy
}

# The inline policy that grants the role its IAM-management permissions. Attached
# as a standalone resource (the aws_iam_role inline_policy block was removed in
# AWS provider v6).
resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example.role.inline_policy
}

# The `p0_aws_iam_write` resource will fail to validate unless it is installed
# _after_ P0's role and its inline policy exist.
resource "p0_aws_iam_write" "example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [aws_iam_role_policy.p0_iam_manager]

  # How users log in to this AWS account. Alternatives to `iam`:
  #   - type = "idc": Identity Center login; also set `parent` (the IDC account
  #     ID) and an `identity` block.
  #   - type = "federated": federated login; set a `provider` block instead of
  #     `identity` (see the p0_aws_iam_write_staged example's federated.tf).
  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}
