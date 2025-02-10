output "app_id" {
  value       = okta_app_saml.aws_account_federation.id
  description = "Application ID of the AWS Account Federation app"
}
