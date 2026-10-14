# Adds Unity Catalog catalogs in an installed workspace, by granting the
# account's service principal MANAGE on each one. Apply it as a Databricks
# account admin who is also the catalog's owner, holds MANAGE on it, or is a
# metastore admin. To add a catalog later, add it to the list and apply again.
#
# To install the connector, account, workspaces and catalogs in one
# configuration that wires them together, see
# https://registry.terraform.io/providers/p0-security/p0/latest/docs/guides/databricks

terraform {
  required_providers {
    # 1.113.0 made databricks_grant honor provider_config with an account-level
    # provider, and 1.115.0 fixed the account-level reads that broke after it.
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
  # The application_id of the account's p0_databricks_account.
  application_id = "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05"
  # Catalog names, in lowercase.
  catalogs = ["main", "analytics"]
}

# The provider must be account-level. It grants through the workspace that
# provider_config names, and looks the workspace up through the account API,
# which only account admins can call. Whoever it signs in as also needs a USER
# assignment to the workspace, which account admins don't get by default, and
# MANAGE on each catalog. Unless told how to sign in, it uses whatever
# Databricks credentials it finds, such as a CLI login, so everyone who plans or
# applies needs their own account admin login. To avoid that, sign it in as a
# service principal with the account_admin role and a federation policy, which
# needs no secret: add auth_type = "env-oidc" and
# client_id = "<its application ID>", and set DATABRICKS_OIDC_TOKEN to an
# identity token that the policy accepts.
provider "databricks" {
  host       = "https://accounts.cloud.databricks.com"
  account_id = local.databricks_account_id
}

# databricks_grant manages only this principal's grants on the catalog. Never
# use databricks_grants here: it overwrites every grant on the catalog.
resource "databricks_grant" "p0_manage" {
  for_each = toset(local.catalogs)

  catalog    = each.key
  principal  = local.application_id
  privileges = ["MANAGE"]

  provider_config {
    workspace_id = local.workspace_id
  }
}

# Each catalog's ID in P0 is "<catalog_name>@<workspace_id>", e.g.
# "main@1234567890123456", because catalog names are unique only within a
# metastore.
resource "p0_databricks_catalog" "example" {
  for_each = toset(local.catalogs)

  workspace_id = local.workspace_id
  catalog_name = each.key
  depends_on   = [databricks_grant.p0_manage]
}
