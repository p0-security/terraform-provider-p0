terraform {
  required_providers {
    okta = {
      source  = "okta/okta"
      version = "4.8.0"
    }
    p0 = {
      source  = "registry.terraform.io/p0-security/p0"
      version = "0.13.0"
    }
  }
}

locals {
  org_url = "https://${var.org_domain}"
}

resource "p0_okta_directory_listing_staged" "p0_api_integration" {
  domain = var.org_domain
}

# To import: terraform import "module.okta_api_integration.okta_app_oauth.p0_api_integration" {applicationId}
resource "okta_app_oauth" "p0_api_integration" {
  label                      = var.app_name
  type                       = "service"
  token_endpoint_auth_method = "private_key_jwt"
  pkce_required              = false
  grant_types                = ["client_credentials"]
  response_types             = ["token"]
  issuer_mode                = "DYNAMIC"
  
  # "Require Demonstrating Proof of Possession (DPoP) header in token requests" must be false.
  # This argument is not supported yet by the Terraform provider, however, the resulting application doesn't enable it when created from Terraform. (Created from the UI it defaults to true.)
  # dpop_bound_access_tokens = false

  jwks {
    kty = p0_okta_directory_listing_staged.p0_api_integration.jwk.kty
    kid = p0_okta_directory_listing_staged.p0_api_integration.jwk.kid
    e   = p0_okta_directory_listing_staged.p0_api_integration.jwk.e
    n   = p0_okta_directory_listing_staged.p0_api_integration.jwk.n
  }
}

output "client_id" {
  value = okta_app_oauth.p0_api_integration.client_id
}


# The scopes provided to the app are limited by the administrative roles assigned to the app. (See further below.)
# To import: terraform import "module.okta_api_integration.okta_app_oauth_api_scope.p0_api_integration_scopes" {applicationId}
resource "okta_app_oauth_api_scope" "p0_api_integration_scopes" {
  app_id = okta_app_oauth.p0_api_integration.id
  issuer = local.org_url # Assumes that the application uses the default org domain
  scopes = [
    # Required for Okta group membership access
    "okta.users.read",
    "okta.groups.manage",
    # Required for AWS resource-based access for Federated user provisioning
    "okta.apps.manage",
    "okta.schemas.manage"
  ]
}

# OAuth scopes alone are not sufficient to perform the administrative tasks P0 needs to perform.
# The administrative roles configuration below allows the following two access types:
# 1) Read Okta users and groups
#   - Requires: custom role with "okta.users.read" and "okta.groups.read" permissions
# 2) AWS resource-based access for Federated user provisioning
#   - Requires: custom role with "okta.apps.manage" permission scoped to only the AWS Account Federation app

# 1) Read Okta users and groups
# To import: terraform import "okta_admin_role_custom.p0_lister_role" {customRoleId}
resource "okta_admin_role_custom" "p0_lister_role" {
  label       = "P0 Directory Lister"
  description = "Allows P0 Security to read all users and all groups"
  permissions = [
    "okta.users.read",
    "okta.groups.read"
  ]
}

# To import: terraform import "okta_resource_set.p0_all_users_groups" {resourceSetId}
resource "okta_resource_set" "p0_all_users_groups" {
  label       = "P0 All Users and Groups"
  description = "All users and all groups"
  resources = [
    "${local.org_url}/api/v1/users",
    "${local.org_url}/api/v1/groups"
  ]
}

# To import: terraform import "okta_app_oauth_role_assignment.p0_lister_role_assignment" {clientId}/{roleAssignmentId}
resource "okta_app_oauth_role_assignment" "p0_lister_role_assignment" {
  type         = "CUSTOM"
  client_id    = okta_app_oauth.p0_api_integration.client_id
  role         = okta_admin_role_custom.p0_lister_role.id
  resource_set = okta_resource_set.p0_all_users_groups.id
}

# The following three resources are for the AWS Okta federation app

# 2) AWS resource-based access for Federated user provisioning
# To import: terraform import "module.okta_api_integration.okta_admin_role_custom.p0_manager_role" {customRoleId}
resource "okta_admin_role_custom" "p0_manager_role" {
  label       = "P0 App Access Manager"
  description = "Allows P0 Security to manage user-to-app assignments and the apps themselves"
  permissions = [
    "okta.users.appAssignment.manage",
    "okta.apps.manage"
  ]
}

# To import: terraform import "module.okta_api_integration.okta_resource_set.p0_managed_resources" {resourceSetId}
resource "okta_resource_set" "p0_managed_resources" {
  label       = "P0 Access Apps"
  description = "List of apps that P0 can grant users access to"
  resources = [
    "${local.org_url}/api/v1/users", # requires all users for the "okta.users.appAssignment.manage" permission
    "${local.org_url}/api/v1/apps/${var.aws_federation_app_id}",
  ]
}

# To import: terraform import "module.okta_api_integration.okta_app_oauth_role_assignment.p0_manager_role_assignment" {clientId}/{roleAssignmentId}
resource "okta_app_oauth_role_assignment" "p0_manager_role_assignment" {
  type         = "CUSTOM"
  client_id    = okta_app_oauth.p0_api_integration.client_id
  role         = okta_admin_role_custom.p0_manager_role.id
  resource_set = okta_resource_set.p0_managed_resources.id
}

resource "p0_okta_directory_listing" "p0_api_integration" {
  client = okta_app_oauth.p0_api_integration.client_id
  domain = p0_okta_directory_listing_staged.p0_api_integration.domain
  jwk    = p0_okta_directory_listing_staged.p0_api_integration.jwk
}
