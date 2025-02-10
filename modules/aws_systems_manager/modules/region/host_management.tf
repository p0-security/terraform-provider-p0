# This is a minimal setup of the AWS Systems Manager that only enables "default host management".
# That is all that's required for P0 Security's SSH access to work.
# AWS Systems Manager offers other features that are not enabled in this Terraform configuration.
# To expand this configuration with other Systems Manager services refer to https://github.com/plus3it/terraform-aws-tardigrade-ssm-default-host-management/blob/main/main.tf

# "Default host management" is enabled by creating an IAM role
# and a service setting that points to the role.
# See discussion: https://github.com/hashicorp/terraform-provider-aws/issues/30474
# In the AWS console, the default host management configuration can be found at
# https://{region}.console.aws.amazon.com/systems-manager/fleet-manager/dhmc

data "aws_region" "current" {}

locals {
  default_host_management_role = trimprefix("${var.default_host_management_role_path}${var.default_host_management_role_name}", "/")

  # Settings of the Quick Start for SSM Default Host Management
  ssm_service_settings = {
    # Default host management service setting
    "/ssm/managed-instance/default-ec2-instance-management-role" : local.default_host_management_role
    # Explorer service settings
    "/ssm/opsitem/ssm-patchmanager" : "Enabled"
    "/ssm/opsitem/EC2" : "Enabled"
    "/ssm/opsdata/ConfigCompliance" : "Enabled"
    "/ssm/opsdata/Association" : "Enabled"
    "/ssm/opsdata/OpsData-TrustedAdvisor" : "Enabled"
    "/ssm/opsdata/ComputeOptimizer" : "Enabled"
    "/ssm/opsdata/SupportCenterCase" : "Enabled"
    "/ssm/opsdata/ExplorerOnboarded" : "true"
  }
}

# To import: terraform import "module.aws_systems_manager.module.region_{regionAlias}.aws_ssm_service_setting.ssm_service_settings[\"{ssmServiceSettingKey}\"]" arn:aws:ssm:{region}:{account}:servicesetting{ssmServiceSettingKey}
resource "aws_ssm_service_setting" "ssm_service_settings" {
  for_each = toset(
    keys(local.ssm_service_settings)
  )

  setting_id    = "arn:aws:ssm:${data.aws_region.current.name}:${local.account_id}:servicesetting${each.key}"
  setting_value = local.ssm_service_settings[each.key]

  lifecycle {
    precondition {
      condition     = can(local.ssm_service_settings[each.key])
      error_message = "The setting \"${each.key}\" is not recognized as a valid SSM service setting for Default Host Management."
    }
  }
}

resource "aws_ssm_association" "update_ssm_agent" {

  name                = "AWS-UpdateSSMAgent"
  association_name    = "UpdateSSMAgent-do-not-delete"
  schedule_expression = "rate(14 days)"

  targets {
    key    = "InstanceIds"
    values = ["*"]
  }
}

