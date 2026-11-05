# Stage the function caller and create its role, as in the
# p0_aws_function_caller_staged example, then install it. P0 checks that the
# role has the inline policy, so install only after the policy exists.
resource "p0_aws_function_caller" "example" {
  id         = p0_aws_function_caller_staged.example.id
  depends_on = [aws_iam_role_policy.p0_function_caller]
}
