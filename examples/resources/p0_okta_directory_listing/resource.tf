resource "p0_okta_directory_listing_staged" "example" {
  domain = "example.okta.com"
}

resource "okta_app_oauth" "p0_api_integration" {
  label                      = "P0 API Integration"
  type                       = "service"
  token_endpoint_auth_method = "private_key_jwt"
  pkce_required              = false
  grant_types                = ["client_credentials"]
  response_types             = ["token"]
  issuer_mode                = "DYNAMIC"

  jwks {
    kty = p0_okta_directory_listing_staged.example.jwk.kty
    kid = p0_okta_directory_listing_staged.example.jwk.kid
    e   = p0_okta_directory_listing_staged.example.jwk.e
    n   = p0_okta_directory_listing_staged.example.jwk.n
  }
}

resource "okta_app_oauth_api_scope" "p0_api_integration_scopes" {
  app_id = okta_app_oauth.p0_api_integration.id
  issuer = "http://example.okta.com"
  scopes = [
    okta.groups.manage,
    okta.users.read
  ]
}


resource "p0_okta_directory_listing" "example" {
  depends_on = [
    p0_okta_directory_listing_staged.example
  ]
  client = p0_okta_directory_listing_staged.example.client
  domain = p0_okta_directory_listing_staged.example.domain
  jwk = {
    kty = p0_okta_directory_listing_staged.example.jwk.kty
    e   = p0_okta_directory_listing_staged.example.jwk.e
    kid = p0_okta_directory_listing_staged.example.jwk.kid
    n   = p0_okta_directory_listing_staged.example.jwk.n
  }
}

