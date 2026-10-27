# Adds a Databricks account: a service principal there that the connector
# exchanges its AWS identity for. The principal is account admin, and metastore
# admin of every metastore in the account through each metastore's admin
# group. Apply it as a Databricks account admin, signed in to the connector's
# AWS account.
# Full chain: p0_databricks_ai_connector -> p0_databricks_ai_account_staged ->
# service principal, federation policy, account admin role and metastore admin
# groups -> p0_databricks_ai_account.

terraform {
  required_providers {
    # 6.36.0 added the aws_iam_outbound_web_identity_federation data source.
    # Referring to the connector's resource instead needs only 6.26.0.
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.36.0"
    }
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
  # The AWS account and region that the connector runs in.
  aws_account_id        = "123456789012"
  aws_region            = "us-east-1"
  databricks_account_id = "01234567-89ab-cdef-0123-456789abcdef"
  accounts_url          = "https://accounts.cloud.databricks.com"

  # The display name of each metastore's admin group. P0's service principal
  # joins each one, which makes it metastore admin. List one group for every
  # metastore in the account.
  metastore_admin_groups = ["metastore-admins"]

  # Who owns what an agent creates once its access ends: a group's display
  # name, a user's userName, or a service principal's application ID.
  owner_principal = "metastore-admins"
}

# Reads the connector's role and issuer, so it must be signed in to the
# connector's AWS account.
provider "aws" {
  region              = local.aws_region
  allowed_account_ids = [local.aws_account_id]
}

# Service principals and their roles live at the account level, so the provider
# must be account-level. Unless told how to sign in, it uses whatever Databricks
# credentials it finds, such as a CLI login, so everyone who plans or applies
# needs their own account admin login. To avoid that, sign it in as a service
# principal with the account_admin role and a federation policy, which needs no
# secret: add auth_type = "env-oidc" and client_id = "<its application ID>", and
# set DATABRICKS_OIDC_TOKEN to an identity token that the policy accepts.
provider "databricks" {
  host       = local.accounts_url
  account_id = local.databricks_account_id
}

# The connector's role, and the issuer of the identity tokens it mints (see the
# p0_databricks_ai_connector example). If this configuration also holds the
# connector, as P0's installer arranges it, refer to the connector's
# aws_iam_role and aws_iam_outbound_web_identity_federation resources instead.
data "aws_iam_role" "connector" {
  name = "p0-connector-databricks-ai"
}

data "aws_iam_outbound_web_identity_federation" "this" {}

resource "p0_databricks_ai_account_staged" "example" {
  id           = local.databricks_account_id
  connector    = local.aws_account_id
  accounts_url = local.accounts_url
}

resource "databricks_service_principal" "p0" {
  display_name = "p0-databricks-ai"
}

# Lets the connector's Lambda role sign in as this principal with an AWS
# identity token, so no secret is ever issued.
resource "databricks_service_principal_federation_policy" "p0" {
  service_principal_id = databricks_service_principal.p0.id
  oidc_policy = {
    issuer    = data.aws_iam_outbound_web_identity_federation.this.issuer_identifier
    subject   = data.aws_iam_role.connector.arn
    audiences = [p0_databricks_ai_account_staged.example.federation_audience]
  }
}

# Account admin lets P0 create a service principal for each agent's request,
# find the account's workspaces, and make itself workspace admin of each one
# an agent uses.
resource "databricks_service_principal_role" "p0_account_admin" {
  service_principal_id = databricks_service_principal.p0.id
  role                 = "account_admin"
}

# Metastore admin lets P0 grant Unity Catalog privileges on any catalog, and
# hand what an agent created to the owner principal when its access ends.
data "databricks_group" "metastore_admins" {
  for_each     = toset(local.metastore_admin_groups)
  display_name = each.value
}

resource "databricks_group_member" "p0_metastore_admin" {
  # Only the IDs, so the group's deprecated attributes stay out of the plan.
  for_each  = { for name, group in data.databricks_group.metastore_admins : name => group.id }
  group_id  = each.value
  member_id = databricks_service_principal.p0.id
}

# Completes the install. P0 checks that the connector can sign in as the
# principal, that the principal is account admin and metastore admin, and that
# owner_principal names one principal in the account.
resource "p0_databricks_ai_account" "example" {
  id              = p0_databricks_ai_account_staged.example.id
  connector       = p0_databricks_ai_account_staged.example.connector
  accounts_url    = p0_databricks_ai_account_staged.example.accounts_url
  application_id  = databricks_service_principal.p0.application_id
  owner_principal = local.owner_principal

  depends_on = [
    databricks_service_principal_federation_policy.p0,
    databricks_service_principal_role.p0_account_admin,
    databricks_group_member.p0_metastore_admin,
  ]
}
