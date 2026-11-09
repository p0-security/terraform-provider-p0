# One Okta AWS federation app. Install the Okta directory integration first: P0 uses it
# to assign AWS roles to users in the app.
resource "p0_aws_federation" "example" {
  id = "okta-prod"
  okta = {
    app_id = "0oa1234567890abcdef"
    # Use "alias-and-role-name" when the app signs in to several AWS accounts.
    role_label_format = "role-name"
  }
}

# An AWS account that users log in to with the federation app.
resource "p0_aws_iam_write" "example" {
  id = p0_aws_iam_write_staged.example.id
  login = {
    type       = "federated"
    federation = p0_aws_federation.example.id
    # Name of the IAM SAML identity provider in this account that trusts the app.
    saml_provider = "okta-saml"
  }
}
