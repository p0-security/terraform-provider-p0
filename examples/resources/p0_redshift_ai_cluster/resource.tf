# Adds one Redshift provisioned cluster to P0. Apply it as an AWS admin of the
# cluster's account, which is also the connector's.
# Full chain: p0_redshift_ai_connector -> p0_redshift_ai_cluster_staged ->
# the connector role's policy for the cluster and the setup SQL ->
# p0_redshift_ai_cluster.
#
# For a Serverless workgroup, set type = "serverless", look the workgroup up
# with the aws_redshiftserverless_workgroup data source, and in the SignIn
# statement allow redshift-serverless:GetCredentials on the workgroup's ARN.
# The Lookup statement then allows redshift-serverless:GetWorkgroup.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0"
    }
    # 0.87.0 added the p0_redshift_ai resources.
    p0 = {
      source  = "p0-security/p0"
      version = ">= 0.87.0"
    }
  }
}

locals {
  account_id = "123456789012"
  # The cluster's region, which may differ from the connector's.
  region = "us-west-2"
  # The provisioned cluster's identifier.
  cluster_identifier = "analytics"
  # The database that the connector's admin statements connect to.
  database = "dev"
}

provider "aws" {
  region              = local.region
  allowed_account_ids = [local.account_id]
}

resource "p0_redshift_ai_cluster_staged" "example" {
  id        = "${local.account_id}/${local.region}/${local.cluster_identifier}"
  connector = local.account_id
  type      = "provisioned"
}

data "aws_redshift_cluster" "this" {
  cluster_identifier = local.cluster_identifier
}

# Lets the connector's role into this cluster: sign in to any of its
# databases, since the connector reads and revokes in each one, run statements
# through the Data API, and look the cluster up. One inline policy per
# cluster, so removing it removes the cluster's access, and the connector's
# own Terraform never changes when a cluster is added.
resource "aws_iam_role_policy" "connector_cluster" {
  name = "p0-redshift-ai-${local.region}-${local.cluster_identifier}"
  role = "p0-connector-redshift-ai-aws"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "SignIn"
        Effect   = "Allow"
        Action   = "redshift:GetClusterCredentialsWithIAM"
        Resource = "arn:aws:redshift:${local.region}:${local.account_id}:dbname:${local.cluster_identifier}/*"
      },
      {
        Sid      = "DataApi"
        Effect   = "Allow"
        Action   = ["redshift-data:ExecuteStatement", "redshift-data:BatchExecuteStatement"]
        Resource = data.aws_redshift_cluster.this.arn
      },
      {
        # The Data API checks that a statement is the caller's own.
        Sid      = "Statements"
        Effect   = "Allow"
        Action   = ["redshift-data:DescribeStatement", "redshift-data:GetStatementResult", "redshift-data:CancelStatement"]
        Resource = "*"
      },
      {
        Sid      = "Lookup"
        Effect   = "Allow"
        Action   = "redshift:DescribeClusters"
        Resource = data.aws_redshift_cluster.this.arn
      },
    ]
  })
}

# Before you apply the resource below, have a superuser run this setup SQL in
# the cluster. It creates the connector's database user, lets it manage users
# and roles, and gives it the permissions revoke needs to find an agent's
# grants and to hand over or drop what an agent created:
#
#   CREATE USER "IAMR:p0-connector-redshift-ai-aws" PASSWORD DISABLE;
#   GRANT ROLE "sys:secadmin" TO "IAMR:p0-connector-redshift-ai-aws";
#   CREATE ROLE p0_connector_admin;
#   GRANT ACCESS SYSTEM TABLE, ALTER TABLE, DROP SCHEMA, DROP FUNCTION, DROP PROCEDURE TO ROLE p0_connector_admin;
#   GRANT ROLE p0_connector_admin TO "IAMR:p0-connector-redshift-ai-aws";

# Completes the install. P0 checks that the connector can find the cluster and
# sign in, that its user holds the setup SQL's roles, and that object_owner
# names an existing user.
resource "p0_redshift_ai_cluster" "example" {
  id        = p0_redshift_ai_cluster_staged.example.id
  connector = p0_redshift_ai_cluster_staged.example.connector
  type      = p0_redshift_ai_cluster_staged.example.type
  database  = local.database
  # Takes over each table and view an agent created once its access ends.
  # Leave it out to have the connector's own user take them over.
  object_owner = "etl_owner"

  depends_on = [aws_iam_role_policy.connector_cluster]
}
