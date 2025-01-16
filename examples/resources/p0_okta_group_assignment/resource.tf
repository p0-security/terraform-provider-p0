resource "okta_app_oauth_role_assignment" "p0_group_membership_admin_role_assignment" {
  type      = "GROUP_MEMBERSHIP_ADMIN"
  client_id = okta_app_oauth.p0_api_integration.client_id
}

resource "p0_okta_group_assignment" "example" {
  depends_on = [
    p0_okta_directory_listing.example
  ]
  domain = "example.okta.com"
}
