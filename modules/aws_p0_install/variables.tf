variable "gcp_service_account_id" {
  description = "The GCP service account ID that P0 uses to access the AWS account"
}

variable "aws_saml_identity_provider_name" {
  description = "The name of the SAML identity provider that the Okta AWS Account Federation app uses"
}
