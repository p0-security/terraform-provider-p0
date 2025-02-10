# Module which deploys the P0 AWS IAM Management Integration (incl. SSH)

terraform {
  required_providers {
    p0 = {
      source  = "registry.terraform.io/p0-security/p0"
      version = "0.13.0"
    }
  }
}

resource "p0_aws_iam_write_staged" "p0-demo-okta" {
  id = var.aws_account_id
}

resource "p0_aws_iam_write" "p0-demo-okta" {
  id         = var.aws_account_id
  depends_on = [p0_aws_iam_write_staged.p0-demo-okta]
  login = {
    type = "federated"
    provider = {
      app_id            = var.aws_okta_federation_app_id
      identity_provider = var.aws_identity_provider
      method = {
        account_count = {
          parent = "533267270629"
          type   = "multi"
        }
      }
    }
  }
}

resource "p0_ssh_aws" "p0-demo-okta" {
  account_id      = var.aws_account_id
  group_key       = var.aws_group_key
  is_sudo_enabled = var.aws_is_sudo_enabled
}