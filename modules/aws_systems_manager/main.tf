terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.42.0"
      configuration_aliases = [
        aws.default,
        aws.us_west_1,
        aws.us_west_2,
      ]
    }
  }
}

locals {
  # Tag resource created by Terraform with the "managed-by"="terraform" tag
  tags = {
    managed-by = "terraform"
  }
}

# The IAM role for host management is shared by all regions
# To import: terraform import "module.aws_systems_manager.aws_iam_role.default_host_management_role" AWSSystemsManagerDefaultEC2InstanceManagementRole
resource "aws_iam_role" "default_host_management_role" {

  provider = aws.default

  name        = "AWSSystemsManagerDefaultEC2InstanceManagementRole"
  path        = "/service-role/"
  description = "AWS Systems Manager Default EC2 Instance Management Role"

  # AmazonSSMManagedEC2InstanceDefaultPolicy is an AWS-managed policy
  managed_policy_arns = ["arn:aws:iam::aws:policy/AmazonSSMManagedEC2InstanceDefaultPolicy"]

  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Sid" : "",
        "Effect" : "Allow",
        "Principal" : {
          "Service" : "ssm.amazonaws.com"
        },
        "Action" : "sts:AssumeRole"
      }
    ]
  })

  tags = local.tags

  lifecycle {
    prevent_destroy = true
  }
}


module "region_us_west_1" {
  source = "./modules/region"
  providers = {
    aws = aws.us_west_1
  }

  enabled_vpcs                      = var.regional_aws["us-west-1"].enabled_vpcs
  default_host_management_role_path = aws_iam_role.default_host_management_role.path
  default_host_management_role_name = aws_iam_role.default_host_management_role.name
}

module "region_us_west_2" {
  source = "./modules/region"
  providers = {
    aws = aws.us_west_2
  }

  enabled_vpcs                      = var.regional_aws["us-west-2"].enabled_vpcs
  default_host_management_role_path = aws_iam_role.default_host_management_role.path
  default_host_management_role_name = aws_iam_role.default_host_management_role.name
}
