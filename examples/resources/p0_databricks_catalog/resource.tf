# Adds Unity Catalog catalogs in an installed workspace, by granting the
# account's service principal MANAGE on each one. Apply it as each catalog's
# owner, a holder of MANAGE on it, or a metastore admin. To add a catalog later,
# add it to the list and apply again.

terraform {
  required_providers {
    databricks = {
      source = "databricks/databricks"
    }
    p0 = {
      source = "p0-security/p0"
    }
  }
}

locals {
  workspace_id = "1234567890123456"
  catalogs     = ["main", "analytics"]
}

# Unity Catalog grants are made in the workspace.
provider "databricks" {
  host = "https://dbc-1234abcd-5678.cloud.databricks.com"
}

# databricks_grant owns this principal's grants on the catalog, and leaves
# everyone else's alone. Never use databricks_grants here: it overwrites every
# principal's grants on the catalog, including the ones P0 makes.
resource "databricks_grant" "p0_manage" {
  for_each = toset(local.catalogs)

  catalog = each.key
  # The application_id of the account's p0_databricks_account.
  principal  = "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05"
  privileges = ["MANAGE"]
}

resource "p0_databricks_catalog" "example" {
  for_each = toset(local.catalogs)

  id         = each.key
  workspace  = local.workspace_id
  depends_on = [databricks_grant.p0_manage]
}
