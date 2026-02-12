# Terraform doesn't support nested loops.
# To work around this limitation a submodule is used which iterates over the required VPC endpoints.
# This dynamic module block iterates over the enabled VPCs and calls the submodule for each VPC.
module "aws_vpc_endpoint" {
  for_each = var.enabled_vpcs

  source = "./modules/vpc_endpoints"

  vpc_id = each.key
  tags   = local.tags
}
