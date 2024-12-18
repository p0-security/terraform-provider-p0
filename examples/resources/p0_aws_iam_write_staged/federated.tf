resource "p0_aws_iam_write_staged" "example_staged" {
  id = "123456789012"
}

resource "p0_aws_iam_write" "example" {
  depends_on = [p0_aws_iam_write_staged.example_staged]
  id         = p0_aws_iam_write_staged.example_staged.id
  login = {
    type = "federated"
    provider = {
      type              = "okta"
      app_id            = "0oabbhzczltTlpEBf697"
      app_id            = "0abcdefghijKlmNOp123"
      identity_provider = "p0_example_okta"
      method = {
        type = "saml"
        account_count = {
          type = "single"
        }
      }
    }
  }
}
