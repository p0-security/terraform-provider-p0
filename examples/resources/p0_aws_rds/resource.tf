# Prerequisite: the account must already be installed via p0_aws_iam_write.
# The p0-security/p0-rds-vpc/aws module grants the P0 IAM-management role the
# VPC-scoped ec2/rds describe permissions the install verifier checks.
module "aws_rds_vpc" {
  source  = "p0-security/p0-rds-vpc/aws"
  version = "0.1.3"

  aws_role_name = "P0RoleIamManager"
  vpc_id        = "vpc-1234567890abcdef0"
}

resource "p0_aws_rds" "example" {
  id         = "vpc-1234567890abcdef0"
  account_id = "123456789012"
  region     = "us-east-1"
  depends_on = [module.aws_rds_vpc]
}
