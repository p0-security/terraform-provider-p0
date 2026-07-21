# Requires the account already installed via p0_aws_iam_write; p0-security/p0-rds-vpc/aws grants
# P0's IAM-management role the VPC-scoped ec2/rds describe perms the install verifier checks.
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
