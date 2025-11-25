# All resources required for resource-based access with P0

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.42.0"
    }
  }
}

locals {
  tags = {
    managed-by = "terraform"
  }
}

# To import: terraform import "module.aws_p0_resource_access.aws_resourceexplorer2_index.example" arn:aws:resource-explorer-2:{region}:{accountId}:index/{indexId}
resource "aws_resourceexplorer2_index" "resource_index" {
  type = var.is_resource_explorer_aggregator ? "AGGREGATOR" : "LOCAL"
  tags = local.tags
}

# To import: terraform import "module.aws_p0_resource_access.aws_resourceexplorer2_index.example" arn:aws:resource-explorer-2:{region}:{accountId}:view/exampleview/e0914f6c-6c27-4b47-b5d4-6b28
resource "aws_resourceexplorer2_view" "default_view" {
  count = var.is_resource_explorer_aggregator ? 1 : 0

  name = "all-resources-p0"

  default_view = true

  included_property {
    name = "tags"
  }

  tags = local.tags

  depends_on = [aws_resourceexplorer2_index.resource_index]
}
