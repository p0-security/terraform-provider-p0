# Adds a workspace in an installed Databricks account, by assigning the
# account's service principal to it: as a workspace user, or as a workspace
# admin if local.enable_workspace_admin is true. Apply it as a Databricks
# account admin, once per workspace. Then add the workspace's catalogs (see
# p0_databricks_catalog).
#
# To install the connector, account, workspaces and catalogs in one
# configuration that wires them together, see
# https://registry.terraform.io/providers/p0-security/p0/latest/docs/guides/databricks

terraform {
  required_providers {
    databricks = {
      source  = "databricks/databricks"
      version = ">= 1.115.0"
    }
    p0 = {
      source = "p0-security/p0"
    }
  }
}

locals {
  databricks_account_id = "01234567-89ab-cdef-0123-456789abcdef"
  workspace_id          = "1234567890123456"
  # If this configuration also holds the workspace's databricks_mws_workspaces,
  # pass its workspace_id, workspace_url and workspace_name straight through.
  workspace_url = "https://dbc-1234abcd-5678.cloud.databricks.com"
  # The workspace's name in P0. Requests show it, and approval rules can match
  # it.
  workspace_name = "analytics-prod"

  # Whether users can request workspace admin on this workspace. P0's service
  # principal is a workspace admin only while this is true.
  enable_workspace_admin = false
}

# Workspace assignments are made at the account level, so the provider must be
# account-level. Unless told how to sign in, it uses whatever Databricks
# credentials it finds, such as a CLI login, so everyone who plans or applies
# needs their own account admin login. To avoid that, sign it in as a service
# principal with the account_admin role and a federation policy, which needs no
# secret: add auth_type = "env-oidc" and client_id = "<its application ID>", and
# set DATABRICKS_OIDC_TOKEN to an identity token that the policy accepts.
provider "databricks" {
  host       = "https://accounts.cloud.databricks.com"
  account_id = local.databricks_account_id
}

# The service principal of the account's p0_databricks_account, found by its
# application_id. If this configuration also holds the account, as P0's
# installer arranges it, refer to its databricks_service_principal instead.
data "databricks_service_principal" "p0" {
  application_id = "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05"
}

# USER is enough for Unity Catalog grants. Only workspace-admin requests need
# ADMIN, which P0 checks when is_workspace_admin_enabled turns on.
resource "databricks_mws_permission_assignment" "p0" {
  workspace_id = local.workspace_id
  principal_id = data.databricks_service_principal.p0.id
  permissions  = [local.enable_workspace_admin ? "ADMIN" : "USER"]
}

resource "p0_databricks_workspace" "example" {
  id                         = local.workspace_id
  account                    = local.databricks_account_id
  workspace_url              = local.workspace_url
  workspace_name             = local.workspace_name
  is_workspace_admin_enabled = local.enable_workspace_admin
  depends_on                 = [databricks_mws_permission_assignment.p0]
}
