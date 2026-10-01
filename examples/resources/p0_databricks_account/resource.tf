# Adds a Databricks account: a service principal there that holds account
# admin, and that the connector exchanges its AWS identity for. Apply it as a
# Databricks account admin, with the aws provider set to the connector's
# account.
# Full chain: p0_databricks_connector -> p0_databricks_account_staged ->
# service principal, federation policy and account admin role ->
# p0_databricks_account. Then add workspaces (see p0_databricks_workspace).

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
  accounts_url          = "https://accounts.cloud.databricks.com"
}

# Service principals and their roles live at the account level.
provider "databricks" {
  host       = local.accounts_url
  account_id = local.databricks_account_id
}

# The connector's AWS account, its role, and the issuer of the identity tokens
# it mints (see the p0_databricks_connector example).
data "aws_caller_identity" "current" {}

data "aws_iam_role" "connector" {
  name = "p0-connector-databricks"
}

data "aws_iam_outbound_web_identity_federation" "this" {}

resource "p0_databricks_account_staged" "example" {
  id           = local.databricks_account_id
  connector    = data.aws_caller_identity.current.account_id
  accounts_url = local.accounts_url
}

resource "databricks_service_principal" "p0" {
  display_name = "p0-connector"
}

# Lets the connector exchange its AWS identity token for this principal's
# Databricks token.
resource "databricks_service_principal_federation_policy" "p0" {
  service_principal_id = databricks_service_principal.p0.id

  oidc_policy = {
    issuer    = data.aws_iam_outbound_web_identity_federation.this.issuer_identifier
    subject   = data.aws_iam_role.connector.arn
    audiences = ["databricks"]
  }
}

# P0 assigns workspace admin and account admin with it.
resource "databricks_service_principal_role" "p0_account_admin" {
  service_principal_id = databricks_service_principal.p0.id
  role                 = "account_admin"
}

# Completes the install.
resource "p0_databricks_account" "example" {
  id             = p0_databricks_account_staged.example.id
  connector      = p0_databricks_account_staged.example.connector
  accounts_url   = p0_databricks_account_staged.example.accounts_url
  application_id = databricks_service_principal.p0.application_id

  depends_on = [
    databricks_service_principal_federation_policy.p0,
    databricks_service_principal_role.p0_account_admin,
  ]
}
