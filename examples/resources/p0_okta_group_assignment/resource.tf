# Stage the P0 Okta directory listing to generate the JWK that P0 uses to
# authenticate to the Okta API.
resource "p0_okta_directory_listing_staged" "example" {
  domain = "example.okta.com"
}

# Create the Okta service application that P0 authenticates as, using the
# P0-generated JWK for private-key-JWT authentication.
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

# Grant the API scopes P0 needs to read the directory and manage group
# membership.
resource "okta_app_oauth_api_scope" "p0_api_integration_scopes" {
  app_id = okta_app_oauth.p0_api_integration.id
  issuer = "https://${p0_okta_directory_listing_staged.example.domain}"
  scopes = [
    "okta.groups.manage",
    "okta.users.read",
  ]
}

# Finalize the directory listing install, wiring P0 to the Okta application.
resource "p0_okta_directory_listing" "example" {
  depends_on = [
    okta_app_oauth_api_scope.p0_api_integration_scopes,
    okta_app_oauth_role_assignment.p0_group_membership_admin_role_assignment
  ]
  client = okta_app_oauth.p0_api_integration.client_id
  domain = p0_okta_directory_listing_staged.example.domain
  jwk    = p0_okta_directory_listing_staged.example.jwk
}

# Grant the "Group Membership Administrator" admin role to the P0 application.
# This role is required for P0 to manage Okta group membership.
resource "okta_app_oauth_role_assignment" "p0_group_membership_admin_role_assignment" {
  type      = "GROUP_MEMBERSHIP_ADMIN"
  client_id = okta_app_oauth.p0_api_integration.client_id
}

# Install P0 for Okta group assignment. This requires the directory listing
# install and the Group Membership Administrator role to already be in place.
resource "p0_okta_group_assignment" "example" {
  depends_on = [
    p0_okta_directory_listing.example,
    okta_app_oauth_role_assignment.p0_group_membership_admin_role_assignment
  ]
  domain = p0_okta_directory_listing.example.domain
}
