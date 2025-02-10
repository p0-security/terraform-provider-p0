data "aws_region" "current" {}

locals {
  endpoints = tomap({
    s3 = {
      service = "com.amazonaws.${data.aws_region.current.name}.s3",
      type    = "Gateway"
    },
    ssm = {
      service = "com.amazonaws.${data.aws_region.current.name}.ssm",
      type    = "Interface"
    },
    ssmmessages = {
      service = "com.amazonaws.${data.aws_region.current.name}.ssmmessages",
      type    = "Interface"
    },
    ec2 = {
      service = "com.amazonaws.${data.aws_region.current.name}.ec2",
      type    = "Interface"
    },
    ec2messages = {
      service = "com.amazonaws.${data.aws_region.current.name}.ec2messages",
      type    = "Interface"
    },
    logs = {
      service = "com.amazonaws.${data.aws_region.current.name}.logs",
      type    = "Interface"
    },
    kms = {
      service = "com.amazonaws.${data.aws_region.current.name}.kms",
      type    = "Interface"
    },
    sts = {
      service = "com.amazonaws.${data.aws_region.current.name}.sts",
      type    = "Interface"
    },
    monitoring = {
      service = "com.amazonaws.${data.aws_region.current.name}.monitoring",
      type    = "Interface"
    }
  })
}

# Retrieve all subnets of the selected VPC. Prerequisites: (see https://docs.aws.amazon.com/vpc/latest/privatelink/create-interface-endpoint.html#prerequisites-interface-endpoints)
# 1) VPC endpoints require DNS support and DNS hostnames to be enabled. Example:
# resource "aws_vpc" "my_vpc" {
#   enable_dns_support   = true
#   enable_dns_hostnames = true
# }
# 2) Security group must enable outbound HTTPS traffic. In this configuration no secruity group is specified, the default security group of the VPC is used.
# One subnet per Availability Zone would suffice, and only private subnets need VPC endpoints,
# but for simplicity deploy to all subnets. Considerations:
# - The default subnet per AZ could be public
# - Identifying public vs. private subnets is not straightforward (depends on routing)
data "aws_vpc" "selected_vpc" {
  id = var.vpc_id
}

data "aws_subnets" "selected_vpc_subnets" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.selected_vpc.id]
  }
}

# To import: terraform import "module.aws_systems_manager.module.region_{regionAlias}.module.aws_vpc_endpoint[\"{vpcId}\"].aws_vpc_endpoint.ssm_vpc_endpoints[\"ssm\"]" {vpcEndpointId}
# Existing endpoints don't necessarily have to be imported because a "duplicate" VPC endpoint deployed by Terraform 
# can exist next to the existing endpoint, for the same subnet. No error will be thrown.
resource "aws_vpc_endpoint" "ssm_vpc_endpoints" {
  for_each = local.endpoints

  vpc_id            = var.vpc_id
  service_name      = each.value.service
  vpc_endpoint_type = each.value.type
  # Only Interface and GatewayLoadBalancer types require subnet IDs. There are no GatewayLoadBalancer types for SSM.
  subnet_ids = each.value.type == "Interface" ? data.aws_subnets.selected_vpc_subnets.ids : null
  # Only Interface require private_dns_enabled = true to allow services within the VPC to automatically use the endpoint.
  private_dns_enabled = each.value.type == "Interface" ? true : null
  # Only Interface require security_group_ids. The default security group of the VPC is used. It must allow outbound HTTPS traffic.
  # security_group_ids = [...]

  tags = var.tags
}
