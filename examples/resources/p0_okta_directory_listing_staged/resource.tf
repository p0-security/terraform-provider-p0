# P0 generates the JWK the service app authenticates with.
resource "p0_okta_directory_listing_staged" "example" {
  domain = "example.okta.com"
}

# Service app P0 reads the directory as, via the staged JWK.
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

# groups.manage is for the downstream p0_okta_group_assignment; users.read for directory listing.
resource "okta_app_oauth_api_scope" "p0_api_integration_scopes" {
  app_id = okta_app_oauth.p0_api_integration.id
  issuer = "https://${p0_okta_directory_listing_staged.example.domain}"
  scopes = [
    "okta.users.read",
    "okta.groups.manage",
  ]
}

# OAuth scopes alone aren't enough; P0 also needs a custom admin role reading all users and groups.
resource "okta_admin_role_custom" "p0_lister_role" {
  label       = "P0 Directory Lister"
  description = "Allows P0 Security to read all users and all groups"
  permissions = [
    "okta.users.read",
    "okta.groups.read",
  ]
}

resource "okta_resource_set" "p0_all_users_groups" {
  label       = "P0 All Users and Groups"
  description = "All users and all groups"
  resources = [
    "https://${p0_okta_directory_listing_staged.example.domain}/api/v1/users",
    "https://${p0_okta_directory_listing_staged.example.domain}/api/v1/groups",
  ]
}

resource "okta_app_oauth_role_assignment" "p0_lister_role_assignment" {
  type         = "CUSTOM"
  client_id    = okta_app_oauth.p0_api_integration.client_id
  role         = okta_admin_role_custom.p0_lister_role.id
  resource_set = okta_resource_set.p0_all_users_groups.id
}

# Finalizes the install; depends_on ensures the scope grants and role assignment exist first.
resource "p0_okta_directory_listing" "example" {
  depends_on = [
    okta_app_oauth_api_scope.p0_api_integration_scopes,
    okta_app_oauth_role_assignment.p0_lister_role_assignment,
  ]
  client = okta_app_oauth.p0_api_integration.client_id
  domain = p0_okta_directory_listing_staged.example.domain
  jwk    = p0_okta_directory_listing_staged.example.jwk
}
