# P0 generates the JWK it authenticates to the Okta API with.
resource "p0_okta_directory_listing_staged" "example" {
  domain = "example.okta.com"
}

# Service app P0 authenticates as, via the staged JWK.
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
  issuer = "https://${p0_okta_directory_listing_staged.example.domain}"
  scopes = [
    "okta.groups.manage",
    "okta.users.read",
  ]
}

# Finalizes the directory-listing install.
resource "p0_okta_directory_listing" "example" {
  depends_on = [
    okta_app_oauth_api_scope.p0_api_integration_scopes,
    okta_app_oauth_role_assignment.p0_group_membership_admin_role_assignment
  ]
  client = okta_app_oauth.p0_api_integration.client_id
  domain = p0_okta_directory_listing_staged.example.domain
  jwk    = p0_okta_directory_listing_staged.example.jwk
}

# GROUP_MEMBERSHIP_ADMIN role is required for P0 to manage Okta group membership.
resource "okta_app_oauth_role_assignment" "p0_group_membership_admin_role_assignment" {
  type      = "GROUP_MEMBERSHIP_ADMIN"
  client_id = okta_app_oauth.p0_api_integration.client_id
}

# Requires the directory-listing install and the GROUP_MEMBERSHIP_ADMIN role already in place.
resource "p0_okta_group_assignment" "example" {
  depends_on = [
    p0_okta_directory_listing.example,
    okta_app_oauth_role_assignment.p0_group_membership_admin_role_assignment
  ]
  domain = p0_okta_directory_listing.example.domain
}
