# Adds Unity Catalog catalogs in an installed workspace, by granting the
# account's service principal MANAGE on each one. Apply it as each catalog's
# owner, a user with MANAGE on it, or a metastore admin. To add a catalog later,
# add it to the list and apply again.

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
  catalogs       = ["main", "analytics"]
}

# The account-level provider grants through the workspace that provider_config
# names.
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

resource "p0_databricks_catalog" "example" {
  for_each = toset(local.catalogs)

  id         = each.key
  workspace  = local.workspace_id
  depends_on = [databricks_grant.p0_manage]
}
