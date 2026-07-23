# Full MySQL (AWS RDS) install chain, in order: p0_aws_iam_write (bootstraps the
# P0RoleIamManager role both modules below require) -> aws_rds_vpc module -> p0_aws_rds ->
# p0_mysql_staged -> p0-connector/p0-db modules -> p0_mysql (completes it).

locals {
  vpc_id = "vpc-0123456789abcdef0"
}

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_subnets" "example" {
  filter {
    name   = "vpc-id"
    values = [local.vpc_id]
  }
}

# The RDS MySQL instance P0 governs. iam_database_authentication_enabled is required:
# the connector authenticates to the database via IAM.
resource "aws_security_group" "mysql" {
  name        = "my-mysql-instance"
  description = "P0-governed MySQL instance"
  vpc_id      = local.vpc_id
}

resource "aws_db_subnet_group" "mysql" {
  name       = "my-mysql-instance"
  subnet_ids = data.aws_subnets.example.ids
}

resource "aws_db_instance" "mysql" {
  identifier                          = "my-mysql-instance"
  engine                              = "mysql"
  engine_version                      = "8.0"
  instance_class                      = "db.t3.micro"
  allocated_storage                   = 20
  storage_encrypted                   = true
  db_subnet_group_name                = aws_db_subnet_group.mysql.name
  vpc_security_group_ids              = [aws_security_group.mysql.id]
  iam_database_authentication_enabled = true
  username                            = "admin"
  manage_master_user_password         = true
  skip_final_snapshot                 = true
}

# Grants P0RoleIamManager the VPC/subnet/RDS read permissions P0's connector needs.
# Must run before the VPC is registered as an aws-rds integration. (The VPC interface
# endpoints are created by the p0-connector module below via setup_vpc_endpoints.)
module "aws_rds_vpc" {
  source  = "p0-security/p0-rds-vpc/aws"
  version = "0.1.3"

  aws_role_name = "P0RoleIamManager"
  vpc_id        = local.vpc_id
}

resource "p0_aws_rds" "example" {
  id         = local.vpc_id
  account_id = data.aws_caller_identity.current.account_id
  region     = data.aws_region.current.name
  depends_on = [module.aws_rds_vpc]
}

# Stage the install. P0 computes hosting.connector_arn, which names the Lambda the
# p0-connector module deploys below.
resource "p0_mysql_staged" "example" {
  id = "my-mysql-instance"
  hosting = {
    type         = "aws-rds"
    instance_arn = aws_db_instance.mysql.arn
    vpc_id       = p0_aws_rds.example.id
  }
  depends_on = [p0_aws_rds.example]
}

# Deploys P0's MySQL connector, distributed as a container image: builds the ECR
# repo/image and provisions the Lambda under the name P0 set in hosting.connector_arn.
module "p0_connector" {
  source  = "p0-security/p0-connector/aws"
  version = "0.5.1"

  aws_role_name       = "P0RoleIamManager"
  aws_services        = ["rds"]
  setup_vpc_endpoints = true

  service            = "mysql"
  service_subnet_ids = data.aws_subnets.example.ids
  vpc_id             = local.vpc_id

  connector_arn = p0_mysql_staged.example.hosting.connector_arn
}

# Grants the connector's Lambda execution role rds-db:connect on the p0_iam_manager DB
# user and opens the connector -> instance security-group path on the MySQL port.
module "p0_mysql_install" {
  source  = "p0-security/p0-db/aws"
  version = "0.3.0"

  rds_instance_arn = aws_db_instance.mysql.arn

  connector_security_group_id = module.p0_connector.connector_security_group.id
  lambda_execution_role_name  = reverse(split("/", reverse(split(":", module.p0_connector.lambda.role))[0]))[0]
}

# Create the DB user P0 authenticates as by running these in the database (AWS IAM auth
# plugin; not manageable via the AWS provider). P0 verifies these privileges at install:
#   CREATE USER p0_iam_manager IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS';
#   GRANT CREATE USER, CREATE ROLE ON *.* TO p0_iam_manager;
#   GRANT ROLE_ADMIN ON *.* TO p0_iam_manager;
#   GRANT ALL PRIVILEGES ON `%`.* TO p0_iam_manager WITH GRANT OPTION;

# Completes the install once the connector is deployed and wired up.
resource "p0_mysql" "example" {
  id         = p0_mysql_staged.example.id
  port       = "3306"
  default_db = "myapp"
  depends_on = [module.p0_connector, module.p0_mysql_install]
}
