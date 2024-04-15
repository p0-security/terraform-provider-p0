resource "p0_aws_iam_write" "example" {
  id         = p0_aws_staged.example.id
  depends_on = [p0_aws_staged.example]
  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}
