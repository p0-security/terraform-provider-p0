# Users log in via AWS Identity Center using a single shared permission set
# per principal. Requires the AWS Identity Center (merged) integration to be
# installed in the account that contains the Identity Center instance (see the
# p0_aws_midc resource).
resource "p0_aws_iam_write" "merged_idc_example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [p0_aws_iam_write_staged.example]
  login = {
    type   = "merged-idc"
    parent = p0_aws_midc.example.id
  }
}
