variable "saml_identity_provider_name" {
  description = "Name of the SAML identity provider in AWS that the P0 roles must trust"
}

variable "role_count" {
  description = "Number of P0 roles to create"
  default     = 10
}