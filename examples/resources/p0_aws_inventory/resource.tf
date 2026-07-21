# Stage: computes the AWS role config P0 needs to inventory this account.
resource "p0_aws_inventory_staged" "example" {
  id = "123456789012"
}

# Trust policy already embeds the P0 service account, so no extra wiring is needed.
resource "aws_iam_role" "p0_inventory" {
  name               = p0_aws_inventory_staged.example.role.name
  assume_role_policy = p0_aws_inventory_staged.example.role.trust_policy
}

resource "aws_iam_role_policy" "p0_inventory" {
  name   = p0_aws_inventory_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_inventory.name
  policy = p0_aws_inventory_staged.example.role.inline_policy
}

# Install only after the role and policy exist; P0 assumes the role during apply.
resource "p0_aws_inventory" "example" {
  id = p0_aws_inventory_staged.example.id
  depends_on = [
    aws_iam_role.p0_inventory,
    aws_iam_role_policy.p0_inventory
  ]
}
