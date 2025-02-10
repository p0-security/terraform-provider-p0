variable "gcp_project_id" {
  description = "The GCP project id"
  type        = string
}

variable "location" {
  description = "The deployment location of the security perimeter"
  type        = string
}

variable "p0_project_id" {
  description = "The project id for the P0 production environment"
  type        = string
}

variable "service_account_email" {
  description = "The P0 service account email"
  type        = string
}