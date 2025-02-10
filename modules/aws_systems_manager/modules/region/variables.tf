variable "enabled_vpcs" {
  description = "Set of VPC IDs to enable AWS Systems Manager VPC endpoints for. The VPCs must have DNS hostnames and DNS resolution enabled. See https://docs.aws.amazon.com/vpc/latest/privatelink/create-interface-endpoint.html"
  type        = set(string)
}

variable "default_host_management_role_path" {
  description = "The role path of the IAM role to use for host management"
}

variable "default_host_management_role_name" {
  description = "Name of IAM role to use for host management"
}
