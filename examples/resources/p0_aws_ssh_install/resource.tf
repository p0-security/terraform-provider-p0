resource "p0_aws_ssh_install" "example" {
  account_id = p0_aws_staged.example.id
  depends_on = [p0_aws_staged.example]
  group_key  = "vegetables"
}
