# Staging computes the AWS role config P0 needs to inventory this account.
# See the p0_aws_inventory example for the full staged -> aws_iam_role -> p0_aws_inventory chain.
resource "p0_aws_inventory_staged" "example" {
  id = "123456789012"
}
