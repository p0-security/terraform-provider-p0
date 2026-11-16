# The federation app's SAML metadata URL, as the app's settings in Okta or PingOne show it
variable "saml_metadata_url" {
  type = string
}

resource "p0_aws_iam_federation_staged" "example" {
  id                = "okta-prod"
  account_id        = "123456789012"
  identity_provider = "okta"
  app               = "0oa1b2c3d4e5f6g7h8i9"
}

data "http" "saml_metadata" {
  url = var.saml_metadata_url
}

resource "aws_iam_saml_provider" "p0_federation" {
  name                   = p0_aws_iam_federation_staged.example.saml_provider_name
  saml_metadata_document = data.http.saml_metadata.response_body
}

resource "aws_iam_role" "p0_federation" {
  name               = p0_aws_iam_federation_staged.example.role.name
  assume_role_policy = p0_aws_iam_federation_staged.example.role.trust_policy
}

resource "aws_iam_role_policy" "p0_federation" {
  name   = p0_aws_iam_federation_staged.example.role.name
  role   = aws_iam_role.p0_federation.name
  policy = p0_aws_iam_federation_staged.example.role.inline_policy
}

resource "aws_iam_role" "p0_federation_pool" {
  count = p0_aws_iam_federation_staged.example.pool_size
  name  = format("%s-%d", p0_aws_iam_federation_staged.example.role.name, count.index)
  path  = p0_aws_iam_federation_staged.example.pool.path
  # IAM refuses a trust policy that names a SAML provider that doesn't exist.
  depends_on         = [aws_iam_saml_provider.p0_federation]
  assume_role_policy = p0_aws_iam_federation_staged.example.pool.trust_policy
}

resource "aws_iam_role_policy" "p0_federation_pool_ssh" {
  count  = p0_aws_iam_federation_staged.example.pool_size
  name   = "P0PolicySharedSSH"
  role   = aws_iam_role.p0_federation_pool[count.index].name
  policy = p0_aws_iam_federation_staged.example.pool.ssh_policy
}

resource "p0_aws_iam_federation" "example" {
  id                = p0_aws_iam_federation_staged.example.id
  account_id        = p0_aws_iam_federation_staged.example.account_id
  partition         = p0_aws_iam_federation_staged.example.partition
  pool_size         = p0_aws_iam_federation_staged.example.pool_size
  identity_provider = p0_aws_iam_federation_staged.example.identity_provider
  app               = p0_aws_iam_federation_staged.example.app
  depends_on = [
    aws_iam_role_policy.p0_federation,
    aws_iam_role_policy.p0_federation_pool_ssh,
  ]
}
