# Stage the Okta directory-listing installation; P0 generates the JWK used
# to authenticate the Okta service app.
resource "p0_okta_directory_listing_staged" "example" {
  domain = "example.okta.com"
}

# Create the Okta service app that P0 uses to read the directory. It
# authenticates with the P0-generated JWK exported by the staged resource.
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

# Grant the scopes P0 needs: reading users for directory listing and managing
# groups for the downstream p0_okta_group_assignment integration.
resource "okta_app_oauth_api_scope" "p0_api_integration_scopes" {
  app_id = okta_app_oauth.p0_api_integration.id
  issuer = "https://${p0_okta_directory_listing_staged.example.domain}"
  scopes = [
    "okta.groups.manage",
    "okta.users.read",
  ]
}

# OAuth scopes alone are not sufficient for the administrative tasks P0 performs.
# Create a custom admin role that can read all users and groups.
resource "okta_admin_role_custom" "p0_lister_role" {
  label       = "P0 Directory Lister"
  description = "Allows P0 Security to read all users and all groups"
  permissions = [
    "okta.users.read",
    "okta.groups.read",
  ]
}

# Scope the custom role to all users and all groups in the organization.
resource "okta_resource_set" "p0_all_users_groups" {
  label       = "P0 All Users and Groups"
  description = "All users and all groups"
  resources = [
    "https://${p0_okta_directory_listing_staged.example.domain}/api/v1/users",
    "https://${p0_okta_directory_listing_staged.example.domain}/api/v1/groups",
  ]
}

# Assign the custom role and resource set to the P0 service app so it can
# list users and groups.
resource "okta_app_oauth_role_assignment" "p0_lister_role_assignment" {
  type         = "CUSTOM"
  client_id    = okta_app_oauth.p0_api_integration.client_id
  role         = okta_admin_role_custom.p0_lister_role.id
  resource_set = okta_resource_set.p0_all_users_groups.id
}

# Complete the installation once the app, its scope grants, and its admin role
# assignment exist.
resource "p0_okta_directory_listing" "example" {
  depends_on = [
    okta_app_oauth_api_scope.p0_api_integration_scopes,
    okta_app_oauth_role_assignment.p0_lister_role_assignment,
  ]
  client = okta_app_oauth.p0_api_integration.client_id
  domain = p0_okta_directory_listing_staged.example.domain
  jwk    = p0_okta_directory_listing_staged.example.jwk
}
