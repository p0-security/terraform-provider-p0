# Full MySQL (AWS RDS) installation chain.
#
# Install order: aws_rds_vpc module (VPC prerequisite) -> p0_aws_rds ->
# p0_mysql_staged -> P0 MySQL connector modules -> p0_mysql (completes the
# install). IAM database authentication is what P0's connector uses to broker
# access, so the RDS instance must have it enabled.

locals {
  vpc_id = "vpc-0123456789abcdef0"
}

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

# Subnets in the target VPC, used for both the RDS instance and the connector.
data "aws_subnets" "example" {
  filter {
    name   = "vpc-id"
    values = [local.vpc_id]
  }
}

# ---------------------------------------------------------------------------
# The RDS MySQL instance P0 governs. iam_database_authentication_enabled is
# required: P0's connector authenticates to the database using IAM.
# ---------------------------------------------------------------------------
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
  db_subnet_group_name                = aws_db_subnet_group.mysql.name
  vpc_security_group_ids              = [aws_security_group.mysql.id]
  iam_database_authentication_enabled = true
  username                            = "admin"
  manage_master_user_password         = true
  skip_final_snapshot                 = true
}

# ---------------------------------------------------------------------------
# Prerequisite: prepare the instance's VPC for P0's RDS connector. This module
# creates the rds-API VPC interface endpoints and grants the P0RoleIamManager
# role the permissions the connector needs, and must run before the VPC is
# registered as a P0 aws-rds integration.
# ---------------------------------------------------------------------------
module "aws_rds_vpc" {
  source  = "p0-security/p0-rds-vpc/aws"
  version = "0.1.3"

  aws_role_name = "P0RoleIamManager"
  vpc_id        = local.vpc_id
}

# ---------------------------------------------------------------------------
# Register the instance's VPC as a P0 aws-rds integration (see this resource's
# description).
# ---------------------------------------------------------------------------
resource "p0_aws_rds" "example" {
  id         = local.vpc_id
  account_id = data.aws_caller_identity.current.account_id
  region     = data.aws_region.current.name
  depends_on = [module.aws_rds_vpc]
}

# ---------------------------------------------------------------------------
# Stage the MySQL installation. connector_arn is computed by P0 and names the
# Lambda connector deployed by the p0-connector module below.
# ---------------------------------------------------------------------------
resource "p0_mysql_staged" "example" {
  id = "my-mysql-instance"
  hosting = {
    type         = "aws-rds"
    instance_arn = aws_db_instance.mysql.arn
    vpc_id       = p0_aws_rds.example.id
  }
  depends_on = [p0_aws_rds.example]
}

# ---------------------------------------------------------------------------
# Deploy P0's MySQL connector. P0 distributes the connector as a container
# image; the p0-security/p0-connector/aws module creates the ECR repository,
# pushes the image, provisions the container-image Lambda under the name P0
# assigned in hosting.connector_arn, sets up the rds-API VPC interface
# endpoints, and grants the P0RoleIamManager role permission to invoke it.
# ---------------------------------------------------------------------------
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

# ---------------------------------------------------------------------------
# Wire the connector to the governed instance: the p0-security/p0-db/aws module
# grants the connector's Lambda execution role rds-db:connect on the
# p0_iam_manager database user and opens the security-group path from the
# connector to the instance on the MySQL port.
# ---------------------------------------------------------------------------
module "p0_mysql_install" {
  source  = "p0-security/p0-db/aws"
  version = "0.3.0"

  rds_instance_arn = aws_db_instance.mysql.arn

  connector_security_group_id = module.p0_connector.connector_security_group.id
  lambda_execution_role_name  = reverse(split("/", reverse(split(":", module.p0_connector.lambda.role))[0]))[0]
}

# ---------------------------------------------------------------------------
# Create the database user P0 authenticates as. These statements run inside the
# database with the AWS IAM auth plugin (not manageable with the AWS Terraform
# provider) and grant the privileges P0 verifies at install time:
#
#   CREATE USER p0_iam_manager IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS';
#   GRANT CREATE USER, CREATE ROLE ON *.* TO p0_iam_manager;
#   GRANT ROLE_ADMIN ON *.* TO p0_iam_manager;
#   GRANT ALL PRIVILEGES ON `%`.* TO p0_iam_manager WITH GRANT OPTION;
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# Complete the installation once the connector is deployed and wired up.
# ---------------------------------------------------------------------------
resource "p0_mysql" "example" {
  id         = p0_mysql_staged.example.id
  port       = "3306"
  default_db = "myapp"
  depends_on = [module.p0_connector, module.p0_mysql_install]
}
