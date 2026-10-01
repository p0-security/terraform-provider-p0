# Adds a workspace in an installed Databricks account, by making the account's
# service principal a workspace admin there. Apply it as a Databricks account
# admin, once per workspace. Then add the workspace's catalogs (see
# p0_databricks_catalog).

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
  databricks_account_id = "01234567-89ab-cdef-0123-456789abcdef"
  workspace_id          = "1234567890123456"
}

# Workspace assignments are made at the account level.
provider "databricks" {
  host       = "https://accounts.cloud.databricks.com"
  account_id = local.databricks_account_id
}

# The service principal of the account's p0_databricks_account, found by its
# application_id.
data "databricks_service_principal" "p0" {
  application_id = "8c5e2e0a-8f0d-4a3e-9d61-3b2f4c7a1e05"
}

resource "databricks_mws_permission_assignment" "p0" {
  workspace_id = local.workspace_id
  principal_id = data.databricks_service_principal.p0.id
  permissions  = ["ADMIN"]
}

resource "p0_databricks_workspace" "example" {
  id         = local.workspace_id
  account    = local.databricks_account_id
  depends_on = [databricks_mws_permission_assignment.p0]
}
