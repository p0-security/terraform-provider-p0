variable "app_name" {
  description = "Name of the P0 OIDC app to create for login"
}

variable "implicit_assignment" {
  default     = false
  description = "Allow implicit user assignment via Federation Broker Mode."
}

variable "app_redirect_uris" {
  description = "Redirect URIs for the P0 OIDC app"
  type        = list(string)
}
