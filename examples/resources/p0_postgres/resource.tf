# Full installation chain for a P0 PostgreSQL (AWS RDS) integration.
#
# Chain: aws-rds VPC integration (p0-rds-vpc module + p0_aws_rds) ->
# RDS PostgreSQL instance -> p0_postgres_staged -> P0 connector + DB grant
# modules -> p0_postgres (finalize).

# --- Existing network the RDS instance lives in ------------------------------
data "aws_vpc" "selected" {
  default = true
}

data "aws_subnets" "private" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.selected.id]
  }
}

# --- Prerequisite: the VPC's AWS RDS integration must be installed in P0 ------
# Required before staging any PostgreSQL instance in this VPC (see p0_aws_rds).
# The p0-rds-vpc module provisions the AWS-side permissions P0's iam-write role
# needs to manage RDS in this VPC; install it before registering the integration.
module "aws_rds_vpc" {
  source  = "p0-security/p0-rds-vpc/aws"
  version = "0.1.3"

  aws_role_name = "P0RoleIamManager"
  vpc_id        = data.aws_vpc.selected.id
}

resource "p0_aws_rds" "example" {
  id         = data.aws_vpc.selected.id
  account_id = "123456789012"
  region     = "us-east-1"
  depends_on = [module.aws_rds_vpc]
}

# --- RDS PostgreSQL instance with IAM database authentication ----------------
resource "aws_db_subnet_group" "postgres" {
  name       = "p0-postgres"
  subnet_ids = data.aws_subnets.private.ids
}

resource "aws_security_group" "db" {
  name   = "p0-postgres-db"
  vpc_id = data.aws_vpc.selected.id
}

resource "aws_db_instance" "postgres" {
  identifier                          = "p0-postgres"
  engine                              = "postgres"
  engine_version                      = "16"
  instance_class                      = "db.t3.micro"
  allocated_storage                   = 20
  db_name                             = "postgres"
  username                            = "p0admin"
  password                            = "change-me-before-apply" # use a secrets manager in production
  iam_database_authentication_enabled = true
  db_subnet_group_name                = aws_db_subnet_group.postgres.name
  vpc_security_group_ids              = [aws_security_group.db.id]
  skip_final_snapshot                 = true
}

# --- Stage the PostgreSQL installation ---------------------------------------
# Staging computes hosting.connector_arn: the ARN of the connector Lambda that
# P0 expects. P0 derives it by convention (p0-pg-<vpc_id>) and verifies the
# Lambda at that exact name, so the connector must be deployed under it.
resource "p0_postgres_staged" "example" {
  id = "my-postgres-instance"
  hosting = {
    type         = "aws-rds"
    instance_arn = aws_db_instance.postgres.arn
    vpc_id       = p0_aws_rds.example.id
  }
}

# --- P0 connector infrastructure ---------------------------------------------
# These are the modules the P0 app generates for an RDS PostgreSQL install.
#
# p0-connector/aws deploys the connector Lambda under the name computed above
# (via connector_arn), wires it into the RDS VPC with VPC endpoints so it can
# reach the AWS and P0 APIs from private subnets, and grants P0's iam-write role
# permission to invoke it.
module "aws_db_connector_install" {
  source  = "p0-security/p0-connector/aws"
  version = "0.5.1"

  aws_role_name       = "P0RoleIamManager"
  aws_services        = ["rds"]
  setup_vpc_endpoints = true

  service            = "pg"
  service_subnet_ids = data.aws_subnets.private.ids
  vpc_id             = data.aws_vpc.selected.id

  connector_arn = p0_postgres_staged.example.hosting.connector_arn
}

# p0-db/aws connects through the connector as the p0_iam_manager database user
# and provisions the user and grants P0 requires (rds_iam + rds_superuser).
module "aws_pg_install" {
  source  = "p0-security/p0-db/aws"
  version = "0.3.0"

  rds_instance_arn = aws_db_instance.postgres.arn

  connector_security_group_id = module.aws_db_connector_install.connector_security_group.id
  lambda_execution_role_name  = reverse(split("/", reverse(split(":", module.aws_db_connector_install.lambda.role))[0]))[0]
}

# --- Finalize the installation -----------------------------------------------
# Completes once the connector and DB grant modules are deployed. hostname and
# state are computed by P0 and must not be set here.
resource "p0_postgres" "example" {
  id         = p0_postgres_staged.example.id
  port       = "5432"
  default_db = "postgres"
  depends_on = [
    module.aws_db_connector_install,
    module.aws_pg_install,
  ]
}
