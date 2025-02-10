variable "aws_account_id" {
  description = "The AWS Account ID"
  type        = string
}

variable "aws_okta_federation_app_id" {
  description = "The ID of the AWS Account Federation app in Okta that P0 manages access to"
  type        = string
}

variable "aws_identity_provider" {
  description = "The name of the AWS Identity Provider used for login"
  type        = string
}

variable "aws_group_key" {
  description = "The tag key used to group AWS instances. Access can be requested, in one request, to all instances with a shared tag value"
  type        = string
}

variable "aws_is_sudo_enabled" {
  description = "If true, users will be able to request sudo access to the instances"
  type        = string
}
