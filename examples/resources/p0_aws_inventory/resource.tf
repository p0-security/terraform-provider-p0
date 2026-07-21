# Stage the inventory component to compute the AWS role configuration that P0
# needs to inventory this account's resources.
resource "p0_aws_inventory_staged" "example" {
  id = "123456789012"
}

# Create the IAM role P0 assumes. The trust policy already embeds the P0
# service account (service_account_id), so no extra wiring is required.
resource "aws_iam_role" "p0_inventory" {
  name               = p0_aws_inventory_staged.example.role.name
  assume_role_policy = p0_aws_inventory_staged.example.role.trust_policy
}

# Attach the read-only inventory permissions to that role.
resource "aws_iam_role_policy" "p0_inventory" {
  name   = p0_aws_inventory_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_inventory.name
  policy = p0_aws_inventory_staged.example.role.inline_policy
}

# Finalize the install only after the role and its policy exist. P0 verifies it
# can assume this role during apply, so the depends_on is required.
resource "p0_aws_inventory" "example" {
  id = p0_aws_inventory_staged.example.id
  depends_on = [
    aws_iam_role.p0_inventory,
    aws_iam_role_policy.p0_inventory
  ]
}
