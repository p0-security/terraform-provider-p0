# Full install chain for a P0 PostgreSQL (AWS RDS) integration:
# p0_aws_iam_write (account + IAM-management role) -> aws-rds VPC integration -> RDS instance
# -> p0_postgres_staged -> connector + DB grant modules -> p0_postgres.

data "aws_vpc" "selected" {
  default = true
}

data "aws_subnets" "private" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.selected.id]
  }
}

# Install the AWS account first. Staging computes the IAM-management role name, trust policy, and
# inline policy P0 needs; create that role, then install p0_aws_iam_write once it exists. Downstream
# modules take the role name from aws_iam_role.p0_iam_manager rather than hardcoding it.
resource "p0_aws_iam_write_staged" "example" {
  id = "123456789012"
}

resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example.role.name
  assume_role_policy = p0_aws_iam_write_staged.example.role.trust_policy
}

resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example.role.inline_policy
}

resource "p0_aws_iam_write" "example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [aws_iam_role_policy.p0_iam_manager]

  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}

# Prerequisite: the VPC's AWS RDS integration must be installed first (see p0_aws_rds).
# p0-rds-vpc grants P0's iam-write role the AWS permissions to manage RDS in this VPC.
module "aws_rds_vpc" {
  source  = "p0-security/p0-rds-vpc/aws"
  version = "0.1.3"

  aws_role_name = aws_iam_role.p0_iam_manager.name
  vpc_id        = data.aws_vpc.selected.id
}

resource "p0_aws_rds" "example" {
  id         = data.aws_vpc.selected.id
  account_id = p0_aws_iam_write.example.id
  region     = "us-east-1"
  depends_on = [module.aws_rds_vpc]
}

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

# Staging computes hosting.connector_arn. P0 derives the connector Lambda name by
# convention (p0-pg-<vpc_id>) and verifies it, so the connector must deploy under that name.
resource "p0_postgres_staged" "example" {
  id = "my-postgres-instance"
  hosting = {
    type         = "aws-rds"
    instance_arn = aws_db_instance.postgres.arn
    vpc_id       = p0_aws_rds.example.id
  }
}

# Deploy the connector with P0's public, published Terraform Registry modules (use these
# as-is; don't build your own). p0-connector/aws deploys the connector Lambda (name from
# connector_arn) with VPC endpoints for AWS/P0 API access, and lets P0's iam-write role invoke it.
module "aws_db_connector_install" {
  source  = "p0-security/p0-connector/aws"
  version = "0.5.1"

  aws_role_name       = aws_iam_role.p0_iam_manager.name
  aws_services        = ["rds"]
  setup_vpc_endpoints = true

  service            = "pg"
  service_subnet_ids = data.aws_subnets.private.ids
  vpc_id             = data.aws_vpc.selected.id

  connector_arn = p0_postgres_staged.example.hosting.connector_arn
}

# p0-db/aws grants the connector's Lambda execution role rds-db:connect on the
# p0_iam_manager DB user and opens the connector -> instance security-group path.
module "aws_pg_install" {
  source  = "p0-security/p0-db/aws"
  version = "0.3.0"

  rds_cluster_arn = aws_db_instance.postgres.arn

  connector_security_group_id = module.aws_db_connector_install.connector_security_group.id
  lambda_execution_role_name  = reverse(split("/", reverse(split(":", module.aws_db_connector_install.lambda.role))[0]))[0]
  depends_on                  = [p0_aws_rds.example]
}

# Create the DB user P0 authenticates as by running this SQL in the database
resource "p0_postgres" "example" {
  id         = p0_postgres_staged.example.id
  port       = "5432"
  default_db = "postgres"
  depends_on = [
    module.aws_db_connector_install,
    module.aws_pg_install,
  ]
}
