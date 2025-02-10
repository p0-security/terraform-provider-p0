terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.42.0"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  account_id = data.aws_caller_identity.current.account_id
  # Tag resource created by Terraform with the "managed-by"="terraform" tag
  tags = {
    managed-by = "terraform"
  }
}
