resource "p0_aws_midc_staged" "example" {
  id         = "123456789012"
  idc_region = "us-east-1"
}

resource "aws_iam_role" "p0_midc_manager" {
  name               = p0_aws_midc_staged.example.role.name
  assume_role_policy = p0_aws_midc_staged.example.role.trust_policy
}

resource "aws_iam_role_policy" "p0_midc_manager" {
  name   = p0_aws_midc_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_midc_manager.name
  policy = p0_aws_midc_staged.example.role.inline_policy
}

resource "p0_aws_midc" "example" {
  id         = p0_aws_midc_staged.example.id
  idc_region = p0_aws_midc_staged.example.idc_region
  partition  = p0_aws_midc_staged.example.partition
  depends_on = [aws_iam_role_policy.p0_midc_manager]
}
