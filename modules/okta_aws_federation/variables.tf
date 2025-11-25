variable "app_name" {
  description = "Name of the AWS Account Federation app to create"
}

variable "aws_account_id" {
  description = "AWS Account ID that the app will federate users into"
}

variable "aws_saml_identity_provider_name" {
  description = "Name of the AWS Identity Provider in the AWS Account used for SAML federation"
}

variable "login_app_client_id" {
  description = "The client_id of the Okta app used for logging to this AWS Account Federation app"
}

variable "enduser_note" {
  description = "Note to end users about the AWS Account Federation app"
}
