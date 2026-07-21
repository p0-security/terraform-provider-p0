# Staging computes the AWS role configuration (role.name, role.trust_policy,
# role.inline_policy, role.inline_policy_name) that P0 needs to inventory this
# account. See the p0_aws_inventory example for the full
# staged -> aws_iam_role -> p0_aws_inventory chain.
resource "p0_aws_inventory_staged" "example" {
  id = "123456789012"
}
