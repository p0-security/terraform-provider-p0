variable "gcp_organization_id" {
  description = "The GCP Organization ID"
  type        = number
}

variable "gcp_project_id" {
  description = "The GCP Project ID"
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

variable "gcp_group_key" {
  description = "The tag key used to group GCP instances. Access can be requested, in one request, to all instances with a shared tag value"
  type        = string
}

variable "gcp_is_sudo_enabled" {
  description = "If true, users will be able to request sudo access to the instances"
  type        = string
}
