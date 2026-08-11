# See the p0_aws_oidc_identity_staged example for the preceding steps
# (staging the identity, and creating the AWS-side OIDC provider and role)
# that this resource depends on.

resource "p0_aws_oidc_identity_staged" "example" {
  id                = "github-actions"
  account_id        = "123456789012"
  oidc_provider_url = "https://token.actions.githubusercontent.com"
  audience          = "https://github.com/my-org"
}

data "tls_certificate" "oidc_provider" {
  url = p0_aws_oidc_identity_staged.example.oidc_provider_url
}

resource "aws_iam_openid_connect_provider" "p0" {
  url             = p0_aws_oidc_identity_staged.example.oidc_provider_url
  client_id_list  = [p0_aws_oidc_identity_staged.example.audience]
  thumbprint_list = [data.tls_certificate.oidc_provider.certificates[0].sha1_fingerprint]
}

resource "aws_iam_role" "p0_oidc_grants" {
  name = "P0OidcGrantsRole-${p0_aws_oidc_identity_staged.example.id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.p0.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${replace(p0_aws_oidc_identity_staged.example.oidc_provider_url, "https://", "")}:aud" = p0_aws_oidc_identity_staged.example.audience
        }
      }
    }]
  })
}

# Finalizes the install; depends_on ensures the AWS-side trust exists first.
resource "p0_aws_oidc_identity" "example" {
  id                = p0_aws_oidc_identity_staged.example.id
  account_id        = p0_aws_oidc_identity_staged.example.account_id
  oidc_provider_url = p0_aws_oidc_identity_staged.example.oidc_provider_url
  audience          = p0_aws_oidc_identity_staged.example.audience
  depends_on        = [aws_iam_openid_connect_provider.p0, aws_iam_role.p0_oidc_grants]
}
