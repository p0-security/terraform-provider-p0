terraform {
  required_providers {
    okta = {
      source  = "okta/okta"
      version = ">= 4.8.0"
    }
  }
}

# To import: terraform import "module.okta_native_login.okta_app_oauth.p0_login" {applicationId}
resource "okta_app_oauth" "p0_login" {
  label                      = var.app_name
  type                       = "native"
  token_endpoint_auth_method = "none"
  pkce_required              = true
  grant_types = [
    "authorization_code",
    "urn:ietf:params:oauth:grant-type:token-exchange", # Token Exchange
    "urn:ietf:params:oauth:grant-type:device_code"     # Device Authorization
  ]
  response_types            = ["code"]
  login_mode                = "DISABLED"
  issuer_mode               = "DYNAMIC"
  auto_key_rotation         = true
  redirect_uris             = var.app_redirect_uris
  post_logout_redirect_uris = []
  logo_uri                  = "https://p0.dev/favicon.ico"
  implicit_assignment       = var.implicit_assignment
  omit_secret               = true # Make sure no secret is generated. See https://registry.terraform.io/providers/okta/okta/latest/docs/resources/app_oauth#omit_secret

  # "Require Demonstrating Proof of Possession (DPoP) header in token requests" must be false.
  # This argument is not support yet by the Terraform provider, however, the resulting application doesn't enable it when created from Terraform. (Created from the UI it defaults to true.)
  # dpop_bound_access_tokens = false
}
