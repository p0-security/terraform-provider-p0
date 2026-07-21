# Stage a PostgreSQL (AWS RDS) installation.
#
# Prerequisite: the instance's VPC must already have an AWS RDS integration
# installed in P0 (see the p0_aws_rds example).
#
# Staging computes hosting.connector_arn, the ARN of the connector Lambda that
# P0 expects. Deploy that connector with the P0 app's Terraform modules
# (p0-security/p0-connector/aws and p0-security/p0-db/aws) before finalizing.
# See the p0_postgres example for the full chain.
resource "p0_postgres_staged" "example" {
  id = "my-postgres-instance"
  hosting = {
    type         = "aws-rds"
    instance_arn = "arn:aws:rds:us-east-1:123456789012:db:my-postgres-instance"
    vpc_id       = "vpc-0123456789abcdef0"
  }
}

# Complete the installation once the connector infrastructure is deployed.
resource "p0_postgres" "example" {
  id         = p0_postgres_staged.example.id
  port       = "5432"
  default_db = "postgres"
}
