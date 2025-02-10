variable "app_name" {
  description = "Name of the P0 API Integration app"
}

variable "org_domain" {
  description = "This is the domain name of your Okta account, for example dev-123456.oktapreview.com."
}

variable "aws_federation_app_id" {
  description = "The ID of the AWS Account Federation app in Okta that P0 manages access to"
}
